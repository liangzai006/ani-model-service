# Model 服务 layout 基线

- 仓库：`ani-kratos-layout`
- layout commit：`fd18422211c741dd5242d2992c3d307d522aa28c`
- 生成器：`scripts/new-service`
- 模块：`github.com/zhangzhe-ctrl/ani-model-service`
- 生成命令：`KRATOS_BIN=/tmp/model-tools/kratos BUF_BIN=/tmp/model-tools/buf scripts/new-service github.com/zhangzhe-ctrl/ani-model-service --parent /root/kubercon`
- 生成结果：无远程 Git remote，Kratos gRPC/admin/health 生命周期骨架已生成。
- 配置 Proto 按项目约定从模板默认的 `internal/conf/v1` 调整到 `api/model/v1`，并同步 Buf module/input 根目录和 Go imports。

## 验证

- `/tmp/model-tools/kratos -v`：`kratos version v3.0.0`
- `/tmp/model-tools/buf --version`：`1.60.0`
- `buf lint`：pass
- `buf generate`：pass
- `go test ./...`：pass
- `go vet ./...`：pass
- `go build ./...`：pass

上述仅证明固定 layout 骨架和配置 Proto 可构建；Model 业务、数据库、Storage 和 IAM 仍未实现。
