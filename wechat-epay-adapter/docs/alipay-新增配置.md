# 本次上线：支付宝扫码需要新增的配置

只列 **本提交之后才出现的项**。微信、易支付商户号/密钥、现有回调根域名 **不用改**。

测试环境对照：

- 适配器：`https://pay-test.allaimo.com`
- new-api 回调根：`https://token-api-test.allaimo.com`
- 易支付商户 ID：`10001`

---

## 1. 适配器环境变量（新增）

写在适配器 `.env`（或 `.deploy/wechat-adapter/.env`），与现有 `WECHAT_*` 并列。

| 变量 | 必填 | 测试环境填写示例 |
| --- | --- | --- |
| `ALIPAY_ENABLED` | 是 | `true`（不接支付宝保持 `false`，进程与微信均不受影响） |
| `ALIPAY_APP_ID` | 开启后必填 | 开放平台应用 ID |
| `ALIPAY_PRIVATE_KEY_FILE` | 开启后必填 | `/run/secrets/alipay_app_private_key.pem` |
| `ALIPAY_ALIPAY_PUBLIC_KEY_FILE` | 开启后必填 | `/run/secrets/alipay_public_key.pem` |
| `ALIPAY_NOTIFY_URL` | 开启后必填 | `https://pay-test.allaimo.com/api/v1/alipay/notify` |
| `ALIPAY_GATEWAY` | 开启后必填 | 正式 `https://openapi.alipay.com/gateway.do`；沙箱用沙箱网关 |
| `ALIPAY_SIGN_TYPE` | 建议写上 | `RSA2` |
| `ALIPAY_SELLER_ID` | 否 | 需要核对卖家账号时再填 |

开启后缺密钥或通知 URL，`/health/ready` 为 503。私钥文件 **不能** 与微信 `WECHAT_MCH_PRIVATE_KEY_FILE` 相同。

---

## 2. 适配器密钥文件（新增挂载）

| 文件 | 内容 |
| --- | --- |
| `alipay_app_private_key.pem` | 应用 RSA 私钥 |
| `alipay_public_key.pem` | 支付宝公钥（验签用） |

只读挂到容器 `/run/secrets/`，不要打进镜像或仓库。

---

## 3. new-api 后台（仅支付方式）

路径：计费 → 支付网关 → Epay。

| 项 | 动作 |
| --- | --- |
| 支付方式 | 确认 JSON 里有 `"type": "alipay"`；没有则加一行。微信 `wxpay` 保留 |
| Epay 端点 / 回调地址 / 商户 ID / 密钥 | **不改**（你现在的测试值已可用） |

---

## 4. 支付宝开放平台（新增能力）

| 项 | 说明 |
| --- | --- |
| 当面付 | 应用开通扫码预下单 |
| 应用公钥 | RSA2，与适配器私钥配对 |
| 异步通知 URL | 与 `ALIPAY_NOTIFY_URL` 完全一致 |

---

## 5. 反代（仅当现网没有通配适配器路径时）

新增路径：

`POST https://pay-test.allaimo.com/api/v1/alipay/notify` → 适配器进程  

若 Nginx 按 `deploy/nginx.conf.example` 逐条放行，必须 **新增** `location = /api/v1/alipay/notify`。整站反代则不用改。

---

## 回滚

1. new-api 支付方式去掉或停用 `alipay`
2. 适配器设 `ALIPAY_ENABLED=false` 后滚动发布  

微信配置与已付支付宝单保留。
