package httpserver

import (
	"fmt"
	"net/http"

	"github.com/QuantumNous/new-api/wechat-epay-adapter/internal/admin"
	"github.com/QuantumNous/new-api/wechat-epay-adapter/internal/alipay"
	"github.com/QuantumNous/new-api/wechat-epay-adapter/internal/config"
	"github.com/QuantumNous/new-api/wechat-epay-adapter/internal/observability"
	"github.com/QuantumNous/new-api/wechat-epay-adapter/internal/order"
	"github.com/QuantumNous/new-api/wechat-epay-adapter/internal/store"
	"github.com/QuantumNous/new-api/wechat-epay-adapter/internal/wechat"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func New(db *gorm.DB, options ...SecurityOptions) *gin.Engine {
	router := gin.New()
	securityOptions := SecurityOptions{}
	if len(options) > 0 {
		securityOptions = options[0]
	}
	if err := applySecurityMiddleware(router, securityOptions); err != nil {
		panic(fmt.Sprintf("invalid trusted proxy configuration: %v", err))
	}
	router.Use(gin.Recovery())
	router.GET(RouteHealthLive, func(context *gin.Context) {
		context.Status(http.StatusNoContent)
	})
	router.GET(RouteHealthReady, func(context *gin.Context) {
		if err := store.New(db).ReadinessProbe(); err != nil {
			context.Status(http.StatusServiceUnavailable)
			return
		}
		if securityOptions.ReadyCheck != nil {
			if err := securityOptions.ReadyCheck(); err != nil {
				context.Status(http.StatusServiceUnavailable)
				return
			}
		}
		context.Status(http.StatusNoContent)
	})
	return router
}

func RegisterMetricsRoute(router *gin.Engine, metrics http.Handler, token string) {
	router.GET(RouteMetrics, MetricsBearer(token), gin.WrapH(metrics))
}

func RegisterSubmitRoute(router *gin.Engine, database *store.Store, appConfig config.Config, wechatClient wechat.Client, alipayClient alipay.Client, metrics *observability.Metrics) error {
	policy, err := order.NewReturnURLPolicy(appConfig.ReturnURLAllowlist, nil)
	if err != nil {
		return err
	}
	notifyPolicy, err := order.NewNotifyURLPolicy(appConfig.NewAPINotifyURLs)
	if err != nil {
		return err
	}
	cashier := NewCashierHandler(database, policy)
	router.GET("/cashier/:access_token", cashier.Show)
	router.GET("/api/v1/cashier/:access_token/status", cashier.Status)
	adminRoutes := router.Group("/api/v1/admin", AdminBearer(appConfig.AdminAPIToken))
	adminHandler := NewAdminHandler(admin.New(database))
	adminRoutes.GET("/orders/:out_trade_no", adminHandler.GetOrder)
	adminRoutes.POST("/orders/:out_trade_no/retry-notification", adminHandler.RetryNotification)

	var nativeOrders *order.NativeOrderService
	if wechatClient != nil {
		verifier, ok := wechatClient.(wechat.NotificationVerifier)
		if !ok {
			return fmt.Errorf("WeChat client does not support notification verification")
		}
		router.POST(RouteWechatNotification, NewWechatNotificationHandler(database, verifier, appConfig).Handle)
		nativeOrders = order.NewNativeOrderService(database, wechatClient)
	}
	handler := NewSubmitHandler(database, appConfig, policy, notifyPolicy, nativeOrders)
	if alipayClient != nil {
		verifier, ok := alipayClient.(alipay.NotificationVerifier)
		if !ok {
			return fmt.Errorf("Alipay client does not support notification verification")
		}
		handler.precreate = order.NewPrecreateService(database, alipayClient)
		router.POST(RouteAlipayNotification, NewAlipayNotificationHandler(database, verifier, appConfig, metrics).Handle)
	}
	router.POST(RouteSubmit, handler.Handle)
	return nil
}
