#!/usr/bin/env bash
# ============================================================
# New-API 全栈一键部署脚本
#   构建: new-api (源码) · wechat-epay-adapter (源码) · mysql · nginx
# ============================================================
set -euo pipefail

SCRIPT_DIR="$(cd "$(dirname "${BASH_SOURCE[0]}")" && pwd)"
cd "$SCRIPT_DIR"

# -------------------- 颜色 --------------------
RED='\033[0;31m'
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
CYAN='\033[0;36m'
NC='\033[0m'

log()   { echo -e "${GREEN}[deploy]${NC} $1"; }
warn()  { echo -e "${YELLOW}[warn]${NC} $1"; }
error() { echo -e "${RED}[error]${NC} $1"; }
step()  { echo -e "\n${CYAN}=== $1 ===${NC}"; }

# -------------------- 前置检查 --------------------
step "检查依赖"
if ! command -v docker &>/dev/null; then
    error "未找到 docker, 请先安装 Docker: https://docs.docker.com/get-docker/"
    exit 1
fi
if ! docker compose version &>/dev/null; then
    error "未找到 docker compose, 请安装 Docker Compose v2+"
    exit 1
fi
log "Docker 与 Compose 已就绪"

# -------------------- 配置文件检查 --------------------
step "检查配置文件"

if [ ! -f .env ]; then
    if [ -f .env.example ]; then
        cp .env.example .env
        warn "已从 .env.example 创建 .env, 请编辑后重新运行: nano .env"
        exit 1
    else
        error "未找到 .env 或 .env.example"
        exit 1
    fi
fi
log ".env 已存在"

if [ ! -f wechat-adapter/.env ]; then
    if [ -f wechat-adapter/.env.example ]; then
        cp wechat-adapter/.env.example wechat-adapter/.env
        warn "已从 wechat-adapter/.env.example 创建 .env (未配置, 将跳过支付适配器)"
    else
        warn "未找到 wechat-adapter/.env, 将跳过支付适配器"
    fi
else
    log "wechat-adapter/.env 已存在"
fi

# -------------------- SSL 证书检查 --------------------
step "检查 SSL 证书"
SSL_CERT=$(grep -E '^SSL_CERT=' .env | cut -d= -f2 | tr -d '\r')
SSL_KEY=$(grep -E '^SSL_KEY=' .env | cut -d= -f2 | tr -d '\r')

if [ -z "$SSL_CERT" ] || [ -z "$SSL_KEY" ]; then
    warn "未在 .env 中找到 SSL_CERT / SSL_KEY, 使用默认值"
    SSL_CERT="fullchain.pem"
    SSL_KEY="privkey.pem"
fi

if [ ! -f "nginx/certs/$SSL_CERT" ] || [ ! -f "nginx/certs/$SSL_KEY" ]; then
    warn "SSL 证书不存在: nginx/certs/$SSL_CERT 或 nginx/certs/$SSL_KEY"
    warn "请将证书文件放入 nginx/certs/ 目录后重试"
    warn "如需临时使用自签证书 (仅测试), 运行:"
    warn "  openssl req -x509 -newkey rsa:2048 -keyout nginx/certs/$SSL_KEY -out nginx/certs/$SSL_CERT -days 365 -nodes -subj '/CN=localhost'"
    exit 1
fi
log "SSL 证书已就绪"

# -------------------- 检测是否部署微信支付适配器 --------------------
step "检查微信支付适配器"
ADAPTER_ENABLED=false
SECRETS_DIR="wechat-adapter/secrets"
REQUIRED_KEYS=("wechat_merchant_private_key.pem" "wechat_pay_public_key.pem")

# 判断条件: .env 无占位符 且 密钥文件都在
if ! grep -q "replace-with" wechat-adapter/.env 2>/dev/null; then
    ALL_KEYS=true
    for key in "${REQUIRED_KEYS[@]}"; do
        if [ ! -f "$SECRETS_DIR/$key" ]; then
            ALL_KEYS=false
            break
        fi
    done
    if [ "$ALL_KEYS" = true ]; then
        ADAPTER_ENABLED=true
        log "微信支付适配器已配置, 将一并部署"
    else
        warn "wechat-adapter/.env 已填写但密钥文件缺失, 跳过适配器"
        warn "缺少文件见: $SECRETS_DIR/"
    fi
else
    warn "微信支付适配器未配置 (.env 中仍有占位符)"
    warn "将仅部署 new-api + MySQL + Nginx (3 服务)"
    warn "后续配置微信支付后, 运行: docker compose --profile payment up -d --build"
fi

# -------------------- 密码检查 --------------------
step "检查密码安全性"
if grep -q "ChangeThis" .env; then
    warn ".env 中检测到默认密码 (ChangeThis...), 请修改后再部署"
    exit 1
fi
if [ "$ADAPTER_ENABLED" = true ] && grep -q "replace-with" wechat-adapter/.env; then
    warn "wechat-adapter/.env 中检测到占位符, 请填写真实值后再部署"
    exit 1
fi
log "密码检查通过"

# -------------------- 处理 nginx 模板 --------------------
# 适配器未部署时, 移除 pay.conf.template 避免 nginx DNS 解析失败
PAY_TEMPLATE="nginx/templates/pay.conf.template"
PAY_TEMPLATE_DISABLED="nginx/templates/pay.conf.template.disabled"
if [ "$ADAPTER_ENABLED" = false ]; then
    if [ -f "$PAY_TEMPLATE" ]; then
        mv "$PAY_TEMPLATE" "$PAY_TEMPLATE_DISABLED"
        log "已暂时禁用 pay.conf 模板 (适配器未部署)"
    fi
else
    if [ -f "$PAY_TEMPLATE_DISABLED" ]; then
        mv "$PAY_TEMPLATE_DISABLED" "$PAY_TEMPLATE"
        log "已恢复 pay.conf 模板"
    fi
fi

# -------------------- 构建与启动 --------------------
step "构建镜像并启动服务"
if [ "$ADAPTER_ENABLED" = true ]; then
    log "开始从源码构建 new-api + wechat-epay-adapter (4 服务)"
    docker compose --profile payment up -d --build
else
    log "开始从源码构建 new-api (3 服务, 无支付适配器)"
    docker compose up -d --build
fi
log "首次构建需要下载前端/Go 依赖, 可能耗时 5-15 分钟"

# -------------------- 等待健康检查 --------------------
step "等待服务就绪"
log "等待 MySQL 健康检查..."
timeout=120
while [ $timeout -gt 0 ]; do
    status=$(docker inspect mysql --format '{{.State.Health.Status}}' 2>/dev/null || echo "starting")
    if [ "$status" = "healthy" ]; then
        log "MySQL 已就绪"
        break
    fi
    echo -n "."
    sleep 5
    timeout=$((timeout - 5))
done
echo ""

if [ "$status" != "healthy" ]; then
    warn "MySQL 未在 120 秒内就绪, 请检查日志: docker compose logs mysql"
fi

log "等待 new-api 健康检查..."
timeout=60
while [ $timeout -gt 0 ]; do
    status=$(docker inspect new-api --format '{{.State.Health.Status}}' 2>/dev/null || echo "starting")
    if [ "$status" = "healthy" ]; then
        log "new-api 已就绪"
        break
    fi
    echo -n "."
    sleep 5
    timeout=$((timeout - 5))
done
echo ""

# -------------------- 完成 --------------------
step "部署完成"
echo ""
docker compose ps
echo ""
NEW_API_DOMAIN=$(grep -E '^NEW_API_DOMAIN=' .env | cut -d= -f2 | tr -d '\r')
PAY_DOMAIN=$(grep -E '^PAY_DOMAIN=' .env | cut -d= -f2 | tr -d '\r')
echo -e "${GREEN}New-API:${NC}    https://$NEW_API_DOMAIN"
if [ "$ADAPTER_ENABLED" = true ]; then
    echo -e "${GREEN}支付适配器:${NC}  https://$PAY_DOMAIN"
else
    echo -e "${YELLOW}支付适配器:${NC}  未部署 (配置后运行: docker compose --profile payment up -d --build)"
fi
echo ""
log "首次访问 https://$NEW_API_DOMAIN 创建管理员账号"
if [ "$ADAPTER_ENABLED" = true ]; then
    log "在管理后台 -> 支付设置 中配置 Epay 网关地址为 https://$PAY_DOMAIN"
fi
log "查看日志: docker compose logs -f"
