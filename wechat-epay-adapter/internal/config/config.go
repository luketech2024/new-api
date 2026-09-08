package config

import (
	"crypto/rsa"
	"crypto/x509"
	"encoding/pem"
	"errors"
	"fmt"
	"math/big"
	"net"
	"net/url"
	"os"
	"strconv"
	"strings"
)

const VerifyModePublicKey = "public_key"

type Config struct {
	DatabaseType                string
	DatabaseDSN                 string
	ListenAddr                  string
	PublicBaseURL               string
	EpayPartnerID               string
	EpayKey                     string
	NewAPINotifyURLs            []string
	ReturnURLAllowlist          string
	MaxOrderAmountYuan          string
	WechatAppID                 string
	WechatMerchantID            string
	WechatCertSerial            string
	WechatPrivateKey            string
	WechatAPIV3Key              string
	WechatNotifyURL             string
	WechatVerifyMode            string
	WechatPublicKeyID           string
	WechatPublicKeyFile         string
	WechatPreviousPublicKeyID   string
	WechatPreviousPublicKeyFile string
	AdminAPIToken               string
	MetricsAPIToken             string
	TrustedProxyCIDRs           []string
	NotificationWorkers         int
	LogLevel                    string
	LogDir                      string
	AlipayEnabled               bool
	AlipayAppID                 string
	AlipayPrivateKeyFile        string
	AlipayPublicKeyFile         string
	AlipayNotifyURL             string
	AlipayGateway               string
	AlipaySellerID              string
	AlipaySignType              string
}

func Load() (Config, error) {
	workers, err := optionalPositiveInt("NOTIFICATION_WORKERS", 2)
	if err != nil {
		return Config{}, err
	}

	config := Config{
		DatabaseType:                strings.ToLower(required("DATABASE_TYPE")),
		DatabaseDSN:                 required("DATABASE_DSN"),
		ListenAddr:                  optional("HTTP_LISTEN_ADDR", ":8080"),
		PublicBaseURL:               required("PUBLIC_BASE_URL"),
		EpayPartnerID:               required("EPAY_PARTNER_ID"),
		EpayKey:                     required("EPAY_KEY"),
		NewAPINotifyURLs:            optionalCSV("NEW_API_NOTIFY_URL"),
		ReturnURLAllowlist:          required("RETURN_URL_ALLOWLIST"),
		MaxOrderAmountYuan:          required("MAX_ORDER_AMOUNT_YUAN"),
		WechatAppID:                 required("WECHAT_APP_ID"),
		WechatMerchantID:            required("WECHAT_MCH_ID"),
		WechatCertSerial:            required("WECHAT_MCH_CERT_SERIAL"),
		WechatPrivateKey:            required("WECHAT_MCH_PRIVATE_KEY_FILE"),
		WechatAPIV3Key:              required("WECHAT_API_V3_KEY"),
		WechatNotifyURL:             required("WECHAT_NOTIFY_URL"),
		WechatVerifyMode:            required("WECHAT_VERIFY_MODE"),
		WechatPublicKeyID:           required("WECHAT_PUBLIC_KEY_ID"),
		WechatPublicKeyFile:         required("WECHAT_PUBLIC_KEY_FILE"),
		WechatPreviousPublicKeyID:   optional("WECHAT_PREVIOUS_PUBLIC_KEY_ID", ""),
		WechatPreviousPublicKeyFile: optional("WECHAT_PREVIOUS_PUBLIC_KEY_FILE", ""),
		AdminAPIToken:               required("ADMIN_API_TOKEN"),
		MetricsAPIToken:             required("METRICS_API_TOKEN"),
		TrustedProxyCIDRs:           optionalCSV("TRUSTED_PROXY_CIDRS"),
		NotificationWorkers:         workers,
		LogLevel:                    optional("LOG_LEVEL", "info"),
		LogDir:                      optional("LOG_DIR", "./logs"),
		AlipayEnabled:               optionalBool("ALIPAY_ENABLED"),
		AlipayAppID:                 optional("ALIPAY_APP_ID", ""),
		AlipayPrivateKeyFile:        optional("ALIPAY_PRIVATE_KEY_FILE", ""),
		AlipayPublicKeyFile:         optional("ALIPAY_ALIPAY_PUBLIC_KEY_FILE", ""),
		AlipayNotifyURL:             optional("ALIPAY_NOTIFY_URL", ""),
		AlipayGateway:               optional("ALIPAY_GATEWAY", "https://openapi.alipay.com/gateway.do"),
		AlipaySellerID:              optional("ALIPAY_SELLER_ID", ""),
		AlipaySignType:              optional("ALIPAY_SIGN_TYPE", "RSA2"),
	}
	if err := config.Validate(); err != nil {
		return Config{}, err
	}
	return config, nil
}

func (c Config) Validate() error {
	for name, value := range map[string]string{
		"DATABASE_DSN":           c.DatabaseDSN,
		"EPAY_PARTNER_ID":        c.EpayPartnerID,
		"EPAY_KEY":               c.EpayKey,
		"RETURN_URL_ALLOWLIST":   c.ReturnURLAllowlist,
		"WECHAT_APP_ID":          c.WechatAppID,
		"WECHAT_MCH_ID":          c.WechatMerchantID,
		"WECHAT_MCH_CERT_SERIAL": c.WechatCertSerial,
		"WECHAT_PUBLIC_KEY_ID":   c.WechatPublicKeyID,
	} {
		if value == "" {
			return fmt.Errorf("%s is required", name)
		}
	}
	if c.DatabaseType != "sqlite" && c.DatabaseType != "mysql" && c.DatabaseType != "postgres" {
		return fmt.Errorf("DATABASE_TYPE must be sqlite, mysql, or postgres")
	}
	for name, value := range map[string]string{
		"PUBLIC_BASE_URL":   c.PublicBaseURL,
		"WECHAT_NOTIFY_URL": c.WechatNotifyURL,
	} {
		if err := requireHTTPSURL(name, value); err != nil {
			return err
		}
	}
	// new-api registers one callback path per business flow (wallet top-up and
	// subscription purchase), so the allowlist has to carry several destinations.
	if len(c.NewAPINotifyURLs) == 0 {
		return errors.New("NEW_API_NOTIFY_URL is required")
	}
	for _, notifyURL := range c.NewAPINotifyURLs {
		if err := requireHTTPSURL("NEW_API_NOTIFY_URL", notifyURL); err != nil {
			return err
		}
	}
	amount, ok := new(big.Rat).SetString(c.MaxOrderAmountYuan)
	if !ok || amount.Sign() <= 0 {
		return errors.New("MAX_ORDER_AMOUNT_YUAN must be a positive decimal")
	}
	if c.WechatVerifyMode != VerifyModePublicKey {
		return fmt.Errorf("WECHAT_VERIFY_MODE must be %q", VerifyModePublicKey)
	}
	if len(c.WechatAPIV3Key) != 32 {
		return errors.New("WECHAT_API_V3_KEY must be 32 bytes")
	}
	if len(c.AdminAPIToken) < 32 || len(c.MetricsAPIToken) < 32 {
		return errors.New("admin and metrics API tokens must each contain at least 32 bytes")
	}
	for _, cidr := range c.TrustedProxyCIDRs {
		if _, _, err := net.ParseCIDR(cidr); err != nil {
			return fmt.Errorf("TRUSTED_PROXY_CIDRS contains invalid CIDR: %s", cidr)
		}
	}
	if err := validatePrivateKeyFile(c.WechatPrivateKey); err != nil {
		return fmt.Errorf("WECHAT_MCH_PRIVATE_KEY_FILE: %w", err)
	}
	if err := validatePublicKeyFile(c.WechatPublicKeyFile); err != nil {
		return fmt.Errorf("WECHAT_PUBLIC_KEY_FILE: %w", err)
	}
	if (c.WechatPreviousPublicKeyID == "") != (c.WechatPreviousPublicKeyFile == "") {
		return errors.New("WECHAT_PREVIOUS_PUBLIC_KEY_ID and WECHAT_PREVIOUS_PUBLIC_KEY_FILE must be configured together")
	}
	if c.WechatPreviousPublicKeyID != "" {
		if c.WechatPreviousPublicKeyID == c.WechatPublicKeyID {
			return errors.New("WECHAT_PREVIOUS_PUBLIC_KEY_ID must differ from WECHAT_PUBLIC_KEY_ID")
		}
		if err := validatePublicKeyFile(c.WechatPreviousPublicKeyFile); err != nil {
			return fmt.Errorf("WECHAT_PREVIOUS_PUBLIC_KEY_FILE: %w", err)
		}
	}
	return c.ValidateAlipay()
}

func (c Config) ValidateAlipay() error {
	if !c.AlipayEnabled {
		return nil
	}
	if c.AlipayAppID == "" {
		return errors.New("ALIPAY_APP_ID is required when ALIPAY_ENABLED is true")
	}
	if c.AlipaySignType != "" && c.AlipaySignType != "RSA2" {
		return errors.New("ALIPAY_SIGN_TYPE must be RSA2")
	}
	if err := requireHTTPSURL("ALIPAY_NOTIFY_URL", c.AlipayNotifyURL); err != nil {
		return err
	}
	if err := requireHTTPSURL("ALIPAY_GATEWAY", c.AlipayGateway); err != nil {
		return err
	}
	if c.AlipayPrivateKeyFile == "" || c.AlipayPrivateKeyFile == c.WechatPrivateKey {
		return errors.New("ALIPAY_PRIVATE_KEY_FILE must be a dedicated Alipay private key file")
	}
	if c.AlipayPublicKeyFile == "" || c.AlipayPublicKeyFile == c.WechatPublicKeyFile {
		return errors.New("ALIPAY_ALIPAY_PUBLIC_KEY_FILE must be a dedicated Alipay public key file")
	}
	if err := validatePrivateKeyFile(c.AlipayPrivateKeyFile); err != nil {
		return fmt.Errorf("ALIPAY_PRIVATE_KEY_FILE: %w", err)
	}
	if err := validatePublicKeyFile(c.AlipayPublicKeyFile); err != nil {
		return fmt.Errorf("ALIPAY_ALIPAY_PUBLIC_KEY_FILE: %w", err)
	}
	return nil
}

func optionalBool(name string) bool {
	value := strings.ToLower(strings.TrimSpace(os.Getenv(name)))
	return value == "true" || value == "1" || value == "yes"
}

func required(name string) string {
	return strings.TrimSpace(os.Getenv(name))
}

func optional(name, fallback string) string {
	value := strings.TrimSpace(os.Getenv(name))
	if value == "" {
		return fallback
	}
	return value
}

func optionalCSV(name string) []string {
	var entries []string
	for _, entry := range strings.Split(os.Getenv(name), ",") {
		if trimmed := strings.TrimSpace(entry); trimmed != "" {
			entries = append(entries, trimmed)
		}
	}
	return entries
}

func optionalPositiveInt(name string, fallback int) (int, error) {
	value := optional(name, strconv.Itoa(fallback))
	result, err := strconv.Atoi(value)
	if err != nil || result < 1 || result > 32 {
		return 0, fmt.Errorf("%s must be an integer between 1 and 32", name)
	}
	return result, nil
}

func requireHTTPSURL(name, value string) error {
	parsed, err := url.Parse(value)
	if err != nil || parsed.Scheme != "https" || parsed.Hostname() == "" || parsed.User != nil {
		return fmt.Errorf("%s must be an absolute HTTPS URL", name)
	}
	return nil
}

func validatePrivateKeyFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return errors.New("must contain PEM data")
	}
	if key, err := x509.ParsePKCS8PrivateKey(block.Bytes); err == nil {
		if _, ok := key.(*rsa.PrivateKey); ok {
			return nil
		}
		return errors.New("must contain an RSA private key")
	}
	if _, err := x509.ParsePKCS1PrivateKey(block.Bytes); err == nil {
		return nil
	}
	return errors.New("must contain an RSA private key")
}

func validatePublicKeyFile(path string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	block, _ := pem.Decode(data)
	if block == nil {
		return errors.New("must contain PEM data")
	}
	publicKey, err := x509.ParsePKIXPublicKey(block.Bytes)
	if err != nil {
		return errors.New("must contain an RSA public key")
	}
	if _, ok := publicKey.(*rsa.PublicKey); !ok {
		return errors.New("must contain an RSA public key")
	}
	return nil
}
