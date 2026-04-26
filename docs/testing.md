# 测试运行说明

## 快速检查

```bash
cd /Users/adward/codes/senti
go test ./...
npm --prefix frontend run build
```

## API/E2E Smoke

默认模式会连接当前本地服务和真实 Kimi 配置，适合验证真实环境，但会受外部模型响应时间影响：

```bash
./scripts/e2e-smoke.sh
```

稳定模式会启动本地 Kimi mock，临时重建 backend 指向 mock，测试结束后自动恢复 backend 原配置：

```bash
./scripts/e2e-smoke.sh --mock
```

包含截图上传和 OCR 链路的完整 smoke：

```bash
./scripts/e2e-smoke.sh --mock --full
```

## 环境变量

- `BASE_URL`：默认 `http://localhost`
- `TEST_USER`：默认 `codex_e2e_smoke`
- `TEST_PASS`：默认 `CodexTest123!`
- `INVITE_CODE`：默认 `codex-e2e-smoke-invite`
- `IMAGE_PATH`：默认 `chat_screenshot_example.png`

示例：

```bash
BASE_URL=http://localhost IMAGE_PATH=chat_screenshot_example.png ./scripts/e2e-smoke.sh --mock --full
```

## 当前已知 XFAIL

- `image OCR cleanup`：对应 `BUG-002`，截图 OCR 结果仍混入日志/前缀噪声。
