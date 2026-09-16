# 开发机完整验证

## 验证环境

本轮使用开发机网络环境执行，允许 Makefile 下载并校验固定版本 Buf 工具；未使用受限沙箱网络。`scripts/verify-source` 已按当前配置生成位置检查 `api/model/v1/conf.pb.go`。

## 结果

```text
make verify
PASS: buf 1.60.0 校验、源码生成稳定性、go mod tidy -diff、go test ./...、go vet ./...、go build ./...、go mod verify、git diff --check
```

运行时测试同时验证未配置 `ANI_DATABASE_DSN` 时 `/readyz` 返回 503，符合外部依赖缺失保持未就绪的要求。
