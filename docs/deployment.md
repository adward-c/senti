# Senti 部署方案

## 目标

- 使用 `Docker Compose` 在单台云服务器部署 `Senti`
- 通过 `Nginx` 暴露网页入口和 API 反向代理
- 使用 `PostgreSQL` 保存分析历史
- 使用 `Docker Volume` 持久化数据库与上传文件

## 容器组成

### 1. `web`

- 基于 `Nginx`
- 承载前端静态资源
- 反向代理 `/api/*` 到 `backend`

### 2. `backend`

- 运行 `Go` API 服务
- 负责文本分析、截图上传、OCR、分析编排、Kimi 调用、存储落库
- 通过挂载卷保存上传文件

### 3. `postgres`

- 保存分析记录
- 初始化表结构由 `db/init.sql` 提供

## 持久化策略

- `postgres_data`：持久化 PostgreSQL 数据目录
- `uploads_data`：持久化聊天截图和中间文件

## 生产环境要求

- Linux 云服务器
- 已安装 `Docker` 和 `Docker Compose`
- 开放端口：`80`，如接 HTTPS 再开放 `443`
- 配置 `.env`

## 生产环境变量

至少需要配置：

```env
POSTGRES_DB=senti
POSTGRES_USER=senti
POSTGRES_PASSWORD=your_strong_password
DATABASE_URL=postgres://senti:your_strong_password@postgres:5432/senti?sslmode=disable
HTTP_ADDR=:8080
CORS_ORIGIN=https://your-domain.com
UPLOAD_DIR=/app/uploads
KIMI_API_KEY=your_kimi_api_key
KIMI_BASE_URL=https://api.moonshot.cn/v1
KIMI_MODEL=moonshot-v1-8k
OCR_LANG=chi_sim+eng
```

## 部署步骤

1. 拉取仓库代码到云服务器
2. 复制 `.env.example` 为 `.env`
3. 填写生产环境变量
4. 执行：

```bash
docker compose up --build -d
```

5. 检查健康状态：

```bash
curl http://127.0.0.1/health
```

## 域名接入建议

### 方案 A：直接用当前 Nginx 容器暴露 80 端口

- 适合 Demo 或内网环境
- 域名 A 记录指向云服务器公网 IP

### 方案 B：服务器外层再加一层反向代理处理 HTTPS

- 适合公网正式访问
- 可使用宿主机 Nginx、Caddy 或云负载均衡做 TLS
- 将 `80/443` 入口代理到 Compose 中的 `web`

## 上线后检查项

- `http://your-domain/health` 返回 `{"status":"ok"}`
- 文本分析可以成功返回结果
- 图片上传后能正常触发 OCR
- `docker compose logs -f` 无持续报错
- 容器重启后历史记录和上传文件仍存在

## 已验证内容

- 前端镜像可构建
- 后端镜像可构建
- PostgreSQL 可启动并初始化
- 本地 `Docker Compose` 可拉起三容器
- 健康检查和文本分析 API 可正常访问

## 未完成项

- 尚未部署到具体云服务器
- 尚未绑定真实公网域名
- 尚未配置 HTTPS 证书
