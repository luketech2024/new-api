package main

import (
	"context"
	"errors"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/QuantumNous/new-api/wechat-epay-adapter/internal/alipay"
	"github.com/QuantumNous/new-api/wechat-epay-adapter/internal/config"
	"github.com/QuantumNous/new-api/wechat-epay-adapter/internal/database"
	"github.com/QuantumNous/new-api/wechat-epay-adapter/internal/delivery"
	"github.com/QuantumNous/new-api/wechat-epay-adapter/internal/httpserver"
	"github.com/QuantumNous/new-api/wechat-epay-adapter/internal/observability"
	"github.com/QuantumNous/new-api/wechat-epay-adapter/internal/order"
	"github.com/QuantumNous/new-api/wechat-epay-adapter/internal/store"
	"github.com/QuantumNous/new-api/wechat-epay-adapter/internal/wechat"
	"github.com/QuantumNous/new-api/wechat-epay-adapter/migrations"
)

func main() {
	if err := run(); err != nil {
		log.Printf("adapter startup failed: %v", err)
		os.Exit(1)
	}
}

func run() error {
	appConfig, err := config.Load()
	if err != nil {
		return err
	}
	db, err := database.Open(appConfig)
	if err != nil {
		return err
	}
	if err := migrations.ApplyInitial(db); err != nil {
		return err
	}
	wechatClient, err := wechat.NewSDKClient(context.Background(), appConfig)
	if err != nil {
		return err
	}
	var alipayClient alipay.Client
	if appConfig.AlipayEnabled {
		client, err := alipay.NewSDKClient(appConfig)
		if err != nil {
			return err
		}
		alipayClient = client
	}
	databaseStore := store.New(db)
	metrics := observability.NewMetrics(databaseStore)
	logger, err := observability.NewLogger(appConfig.LogLevel, appConfig.LogDir)
	if err != nil {
		return err
	}
	router := httpserver.New(db, httpserver.SecurityOptions{
		TrustedProxies: appConfig.TrustedProxyCIDRs, RequestObserver: metrics, RequestLogger: logger,
		ReadyCheck: appConfig.ValidateAlipay,
	})
	httpserver.RegisterMetricsRoute(router, metrics, appConfig.MetricsAPIToken)
	if err := httpserver.RegisterSubmitRoute(router, databaseStore, appConfig, wechatClient, alipayClient, metrics); err != nil {
		return err
	}

	server := &http.Server{
		Addr:              appConfig.ListenAddr,
		Handler:           router,
		ReadHeaderTimeout: 5 * time.Second,
		ReadTimeout:       15 * time.Second,
		WriteTimeout:      15 * time.Second,
		IdleTimeout:       60 * time.Second,
	}
	shutdownContext, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()
	for workerNumber := 0; workerNumber < appConfig.NotificationWorkers; workerNumber++ {
		worker, err := delivery.NewWorker(databaseStore, appConfig, fmt.Sprintf("notification-worker-%d", workerNumber+1), nil)
		if err != nil {
			return err
		}
		go worker.Run(shutdownContext)
	}
	var precreate *order.PrecreateService
	if alipayClient != nil {
		precreate = order.NewPrecreateService(databaseStore, alipayClient)
	}
	go order.NewRecoveryScheduler(databaseStore, order.NewNativeOrderService(databaseStore, wechatClient), precreate).Run(shutdownContext)

	errChan := make(chan error, 1)
	go func() {
		errChan <- server.ListenAndServe()
	}()

	select {
	case err := <-errChan:
		if errors.Is(err, http.ErrServerClosed) {
			return nil
		}
		return err
	case <-shutdownContext.Done():
		context, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		defer cancel()
		return server.Shutdown(context)
	}
}
