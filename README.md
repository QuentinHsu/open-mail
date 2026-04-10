# open-mail

一个使用 Go 编写的 Telegram Bot + Outlook IMAP 邮箱监听服务。

它支持：
- 多个 Outlook 邮箱账号接入
- Telegram 指令增删改查邮箱
- REST API 增删改查邮箱
- 新邮件到达后自动推送主题、发件人和正文摘要到 Telegram
- Docker / Docker Compose 部署
- 本地持久化加密保存邮箱密码

## 架构

- Telegram Bot: 管理入口和消息通知出口
- REST API: `GET/POST/PUT/DELETE /v1/mailboxes`
- Mailbox Service: 负责邮箱配置校验、加密、持久化
- Monitor Manager: 每个邮箱一个轮询协程，按 `POLL_INTERVAL` 监听 IMAP 新邮件
- JSON Store: 把邮箱配置和最后处理 UID 落到 `.data/mailboxes.json`

## 重要限制

这个服务当前通过 Outlook IMAP 使用“邮箱 + 密码”登录。是否能成功，取决于你的 Microsoft 账号配置：
- 企业或学校账号通常需要管理员允许 IMAP 基础认证
- 开启 MFA 的账号通常需要应用专用密码，否则账号密码本身可能无法直接登录
- 如果你的租户禁用了 IMAP/基础认证，这种模式就会失败；那时需要改成 OAuth2 / Microsoft Graph

也就是说，这个项目已经把“帐密直接登录”的服务端能力做好了，但能否连上，最终受 Outlook 账号策略约束。

## 环境变量

复制 `.env.example` 为 `.env`，然后填写：

```env
PORT=3000
TELEGRAM_BOT_TOKEN=your-telegram-bot-token
TELEGRAM_ALLOWED_CHAT_IDS=123456789
MAILBOX_ENCRYPTION_KEY=replace-with-a-long-random-secret
DATA_DIR=.data
POLL_INTERVAL=1m
DEFAULT_IMAP_HOST=outlook.office365.com
DEFAULT_IMAP_PORT=993
DEFAULT_IMAP_TLS=true
API_TOKEN=
```

## Telegram 命令

```text
/mailboxes
/add 邮箱 密码 [显示名]
/update ID 邮箱 密码 [显示名]
/remove ID
```

## HTTP API

### 健康检查

```bash
curl http://localhost:3000/v1/health
```

### 创建邮箱

```bash
curl -X POST http://localhost:3000/v1/mailboxes \
  -H 'Content-Type: application/json' \
  -d '{
    "email": "user@outlook.com",
    "password": "your-password",
    "display_name": "main"
  }'
```

### 更新邮箱

```bash
curl -X PUT http://localhost:3000/v1/mailboxes/<id> \
  -H 'Content-Type: application/json' \
  -d '{
    "email": "user@outlook.com",
    "password": "new-password",
    "display_name": "main"
  }'
```

### 删除邮箱

```bash
curl -X DELETE http://localhost:3000/v1/mailboxes/<id>
```

如果设置了 `API_TOKEN`，请求时需要附带：

```bash
-H 'Authorization: Bearer your-token'
```

## 本地运行

```bash
go mod tidy
go run ./cmd/open-mail
```

## Docker 部署

```bash
docker compose up --build -d
```

容器会通过两种方式读取同一份根目录 `.env`：
- `env_file` 把变量注入进程环境
- `./.env:/app/.env:ro` 让容器内也能读取同一份文件

当你修改 `.env` 后，重新执行下面的命令让容器重新创建：

```bash
docker compose up -d --force-recreate
```

## 后续行为

- 新增邮箱时会立即校验 IMAP 登录
- 首次成功接入时，会把当前邮箱最新 UID 作为基线，不会把历史邮件全部回推
- 之后只推送新增邮件
