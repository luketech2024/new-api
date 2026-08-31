# New-API 全栈一键部署

从源码构建镜像，一键部署 **new-api · wechat-epay-adapter (可选) · MySQL · Redis · Nginx** 五个服务。

## 目录结构

```
deploy/
├── docker-compose.yml           # 统一编排 (5 服务)
├── .env.example                  # 主环境变量模板
├── .gitignore                    # 排除敏感文件
├── deploy.sh                     # 一键部署脚本 (含安全检查)
├── README.md                     # 本文件
├── nginx/
│   ├── nginx.conf                # Nginx 主配置
│   ├── templates/                # envsubst 模板 (自动渲染为 conf.d)
│   │   ├── new-api.conf.template
│   │   └── pay.conf.template
│   ├── certs/                    # SSL 证书 (需放入)
│   └── html/                     # 静态文件 (可选)
├── mysql/
│   ├── my.cnf                    # MySQL 配置 (调优)
│   └── init.sh                   # 初始化脚本 (创建双数据库)
└── wechat-adapter/
    ├── .env.example              # 适配器环境变量模板
    └── secrets/                  # 微信密钥 PEM (需放入)
```

## 架构

```
                      ┌─────────────┐
   用户 ──HTTPS──►    │   Nginx     │ :80/:443
                      └──────┬──────┘
                ┌────────────┼────────────┐
                ▼            ▼
       api.example.com   pay.example.com (可选)
                │            │
                ▼            ▼
        ┌───────────┐  ┌─────────────────────┐
        │  new-api  │  │ wechat-epay-adapter │
        │  :3000    │  │  :8080 (profile:payment) │
        └─────┬─────┘  └──────────┬──────────┘
              │                   │
        ┌─────┴─────┐       ┌─────┴─────┐
        ▼           ▼       ▼
  ┌──────────┐ ┌────────┐
  │ Redis 7  │ │ MySQL 8│ :3306
  │  :6379   │ │new_api │
  └──────────┘ │wechat_epay│
               └──────────┘
```

- **new-api** 与 **wechat-epay-adapter** 从本地源码构建 (`Dockerfile` 多阶段构建)
- **MySQL** 使用官方 `mysql:8.0`，一个实例承载 `new_api` + `wechat_epay` 两个数据库
- **Redis** 使用官方 `redis:7-alpine`，提供缓存 / 限流 / 会话协调 / 性能指标
- **Nginx** 使用官方 `nginx:1.27-alpine`，通过 `envsubst` 模板自动渲染站点配置
- **wechat-epay-adapter** 标记为 `profiles: ["payment"]`，不配置微信支付时自动跳过

## 前置要求

- Linux 服务器 (推荐 4GB+ 内存)
- Docker 24+ 与 Docker Compose v2+
- 两个已解析到服务器的域名：
  - `api.example.com` — new-api 前端
  - `pay.example.com` — 微信支付适配器（可选，不配微信支付可省略）
- SSL 证书 (Let's Encrypt 免费证书即可)
- 微信商户平台密钥文件（可选，不配微信支付可省略）

## 快速部署（不含微信支付）

只需 3 步，部署 new-api + MySQL + Redis + Nginx：

```bash
# 1. 编辑主配置
cd deploy
cp .env.example .env
nano .env          # 改域名、密码、SSL 证书名

# 2. 放入 SSL 证书
cp /path/to/fullchain.pem nginx/certs/
cp /path/to/privkey.pem  nginx/certs/

# 3. 一键部署
chmod +x deploy.sh
./deploy.sh        # 自动检测到适配器未配置，部署 4 服务
```

## 完整部署（含微信支付）

### 1. 克隆源码到服务器

```bash
git clone <仓库地址> /opt/new-api
cd /opt/new-api/deploy
```

### 2. 编辑主配置

```bash
cp .env.example .env
nano .env
```

必改项：

| 变量 | 说明 |
|------|------|
| `NEW_API_DOMAIN` | new-api 域名 (如 `api.example.com`) |
| `PAY_DOMAIN` | 支付适配器域名 (如 `pay.example.com`) |
| `SSL_CERT` / `SSL_KEY` | `nginx/certs/` 下的证书文件名 |
| `MYSQL_ROOT_PASSWORD` | MySQL root 密码（自定义） |
| `NEW_API_DB_PASSWORD` | new-api 数据库密码（自定义） |
| `WECHAT_DB_PASSWORD` | 适配器数据库密码（自定义） |
| `REDIS_PASSWORD` | Redis 密码（自定义） |
| `SESSION_COOKIE_SECURE` | 生产 HTTPS 设为 `true` |
| `SESSION_COOKIE_TRUSTED_URL` | 设为 `https://api.example.com` |
| `GENERATE_DEFAULT_TOKEN` | 注册用户是否自动生成 Token，默认 `false` |

### 3. 编辑适配器配置

```bash
cp wechat-adapter/.env.example wechat-adapter/.env
nano wechat-adapter/.env
```

必改项：

| 变量 | 来源 |
|------|------|
| `PUBLIC_BASE_URL` | `https://pay.example.com` |
| `NEW_API_NOTIFY_URL` | 逗号分隔，钱包充值 `https://api.example.com/api/user/epay/notify` 与订阅购买 `https://api.example.com/api/subscription/epay/notify` 需同时列出 |
| `WECHAT_NOTIFY_URL` | `https://pay.example.com/api/v1/wechat/notify` |
| `EPAY_PARTNER_ID` / `EPAY_KEY` | 自己定义（new-api 后台支付设置需一致） |
| `WECHAT_APP_ID` / `WECHAT_MCH_ID` | [微信商户平台](https://pay.weixin.qq.com) → 账户中心 → 商户信息 |
| `WECHAT_MCH_CERT_SERIAL` | 商户平台 → 账户中心 → API安全 → API证书 → 证书序列号 |
| `WECHAT_API_V3_KEY` | 商户平台 → 账户中心 → API安全 → APIv3密钥（32 字节） |
| `WECHAT_PUBLIC_KEY_ID` | 商户平台 → 账户中心 → API安全 → 微信支付公钥 → 公钥ID |
| `ADMIN_API_TOKEN` / `METRICS_API_TOKEN` | `openssl rand -hex 32` 随机生成（各 32+ 字节） |

### 4. 放入证书与密钥

```bash
# SSL 证书
cp /path/to/fullchain.pem nginx/certs/
cp /path/to/privkey.pem  nginx/certs/

# 微信密钥（从商户平台下载）
cp /path/to/apiclient_key.pem  wechat-adapter/secrets/wechat_merchant_private_key.pem
cp /path/to/wechat_pubkey.pem  wechat-adapter/secrets/wechat_pay_public_key.pem
```

### 5. 一键部署

```bash
chmod +x deploy.sh
./deploy.sh
```

`deploy.sh` 会自动检测微信支付是否已配置：
- 已配置（`.env` 无占位符 + 密钥文件存在）→ 部署全部 5 服务 (`--profile payment`)
- 未配置 → 部署 4 服务（new-api + MySQL + Redis + Nginx），跳过适配器

后续配置好微信支付后，再次运行 `./deploy.sh` 即可补上适配器。

### 手动操作 (不用脚本)

```bash
# 不含支付适配器
docker compose up -d --build

# 含支付适配器
docker compose --profile payment up -d --build
```

## 部署后配置

### 初始化 new-api

1. 浏览器访问 `https://api.example.com`
2. 首次进入会提示创建管理员账号
3. 登录管理后台

### 配置支付（已部署适配器时）

在管理后台 **设置 → 支付设置**：

1. **支付地址** 填入适配器公网 URL：`https://pay.example.com`
2. **易支付商户 ID** 填入 `EPAY_PARTNER_ID`
3. **易支付密钥** 填入 `EPAY_KEY`
4. 保存后用户即可通过微信扫码充值

### 支付流程

```
用户发起充值
    → new-api 生成 Epay 表单, 浏览器跳转到 pay.example.com/submit.php
    → wechat-epay-adapter 创建微信 Native 订单, 展示收银台二维码
    → 用户扫码支付
    → 微信异步通知 pay.example.com/api/v1/wechat/notify
    → adapter 验签后回调 api.example.com/api/user/epay/notify
    → new-api 到账, 余额更新
```

## 密码说明

所有密码都是**自定义**的 — MySQL、Redis 都是全新容器，你在 `.env` 里写什么密码，容器启动时就用什么密码创建账户：

| 变量 | 自定义 | 说明 |
|------|--------|------|
| `MYSQL_ROOT_PASSWORD` | 是 | MySQL root 密码 |
| `NEW_API_DB_PASSWORD` | 是 | new-api 数据库用户密码 |
| `WECHAT_DB_PASSWORD` | 是 | 适配器数据库用户密码 |
| `REDIS_PASSWORD` | 是 | Redis 访问密码 |
| `EPAY_PARTNER_ID` / `EPAY_KEY` | 是 | new-api 与 adapter 之间的对接凭据，两边必须一致 |
| `ADMIN_API_TOKEN` / `METRICS_API_TOKEN` | 是 | `openssl rand -hex 32` 生成 |

> 首次启动后改密码不生效（密码已写入数据卷），需 `docker compose down -v` 删除数据卷后重建。

## 环境变量说明

### deploy/.env（主配置，Docker Compose 变量替换）

| 变量 | 默认值 | 说明 |
|------|--------|------|
| `NEW_API_DOMAIN` | `api.example.com` | new-api 前端域名 |
| `PAY_DOMAIN` | `pay.example.com` | 支付适配器域名 |
| `SSL_CERT` / `SSL_KEY` | `fullchain.pem` / `privkey.pem` | nginx/certs/ 下证书文件名 |
| `MYSQL_ROOT_PASSWORD` | — | MySQL root 密码 |
| `NEW_API_DB_NAME` / `_USER` / `_PASSWORD` | `new_api` / `newapi` / — | new-api 数据库 |
| `WECHAT_DB_NAME` / `_USER` / `_PASSWORD` | `wechat_epay` / `wechat_epay` / — | 适配器数据库 |
| `REDIS_PASSWORD` | — | Redis 密码 |
| `SESSION_COOKIE_SECURE` | `false` | 生产 HTTPS 设 `true` |
| `SESSION_COOKIE_TRUSTED_URL` | — | Secure=true 时必填的 HTTPS Origin |
| `GENERATE_DEFAULT_TOKEN` | `false` | 注册用户是否自动生成 API Token |

### deploy/wechat-adapter/.env（适配器配置，env_file 加载到容器）

微信商户平台获取的凭据，详见 [wechat-adapter/.env.example](wechat-adapter/.env.example) 注释。

> 注意：`DATABASE_TYPE` / `DATABASE_DSN` / `HTTP_LISTEN_ADDR` 由 docker-compose.yml 的 `environment` 统一管理，不在 `.env` 中设置。

## 常用运维命令

```bash
# 查看服务状态
docker compose ps

# 查看日志
docker compose logs -f
docker compose logs -f new-api
docker compose logs -f wechat-epay-adapter

# 重新构建 (代码更新后)
docker compose up -d --build new-api
docker compose --profile payment up -d --build wechat-epay-adapter

# 重启单个服务
docker compose restart new-api

# 停止所有服务
docker compose down

# 停止并清除数据 (危险! 会丢失所有数据!)
docker compose down -v

# MySQL 备份
docker exec mysql mysqldump -uroot -p"$MYSQL_ROOT_PASSWORD" --single-transaction --routines --events new_api > backup_new_api.sql
docker exec mysql mysqldump -uroot -p"$MYSQL_ROOT_PASSWORD" --single-transaction --routines --events wechat_epay > backup_wechat_epay.sql

# MySQL 恢复
docker exec -i mysql mysql -uroot -p"$MYSQL_ROOT_PASSWORD" new_api < backup_new_api.sql

# Redis 备份 (RDB 快照)
docker exec redis redis-cli -a "$REDIS_PASSWORD" SAVE
docker cp redis:/data/dump.rdb ./redis_backup.rdb

# 补充部署支付适配器 (之前没配，后来配好了)
nano wechat-adapter/.env          # 填写微信商户信息
cp wechat_merchant_private_key.pem wechat_pay_public_key.pem wechat-adapter/secrets/
./deploy.sh                        # 自动检测到已配置，部署 5 服务
```

## 行尾处理 (Windows 部署时)

如果是 Windows 环境传文件到 Linux 服务器，确保 shell 脚本和配置文件使用 LF 行尾：

```bash
# 在服务器上转换所有 deploy 文件
find deploy/ -type f \( -name '*.sh' -o -name '*.cnf' -o -name '*.conf' -o -name '*.template' -o -name '*.yml' -o -name '.env' -o -name '.env.example' \) -exec sed -i 's/\r$//' {} +

# 或只转换 .env 文件
sed -i 's/\r$//' .env wechat-adapter/.env
```

或使用 `git clone`（项目 `.gitattributes` 已配置 LF）。

> `deploy.sh` 已做 CRLF 防护，解析 `.env` 变量时自动 `tr -d '\r'`。

## 注意事项

- **密码安全**：`.env` 中的密码不要使用默认值，`deploy.sh` 会检测 `ChangeThis` 并拒绝部署
- **SSL 证书**：生产环境必须使用受信任的 CA 证书；微信回调要求 HTTPS
- **微信密钥**：PEM 文件放在 `wechat-adapter/secrets/` 目录，以只读方式挂载到 `/run/secrets`
- **数据持久化**：MySQL、Redis、new-api 数据/日志使用 Docker named volumes，重建容器不丢数据
- **内存限制**：MySQL 默认限制 4GB；服务器内存不足时调低 `my.cnf` 中的 `innodb_buffer_pool_size`
- **Redis**：提供缓存、限流、会话协调、性能指标；单节点部署可用内存缓存替代，但功能受限
- **网络隔离**：MySQL 端口 3306 映射到宿主机方便管理，生产环境建议改为 `127.0.0.1:3306:3306`
- **适配器可选**：`wechat-epay-adapter` 标记为 `profiles: ["payment"]`，不配置时自动跳过

## 与参考服务器的差异

参考服务器 (`47.97.201.3`) 的现有部署与本方案的差异：

| 项目 | 参考服务器 | 本方案 |
|------|-----------|--------|
| new-api 镜像 | 官方镜像 (Aliyun) | 源码构建 |
| new-api 数据库 | SQLite (`/data`) | MySQL |
| nginx | 自定义镜像 | 官方 `nginx:1.27-alpine` |
| nginx → new-api | `host.docker.internal:3000` | 容器名 `new-api:3000` |
| 适配器镜像 | 源码构建 | 源码构建 (相同) |
| MySQL | 独立 compose | 统一 compose |
| Redis | 无 | `redis:7-alpine` |
| 适配器 | 必选 | 可选 (profiles) |
