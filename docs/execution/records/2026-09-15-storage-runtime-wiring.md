# Storage gRPC runtime wiring

组合根现在读取 `ANI_STORAGE_GRPC_ADDR` 创建版本化 Storage gRPC client，并将其注入 Model 上传/下载 handler。默认不连接任何地址；可选 `ANI_STORAGE_GRPC_INSECURE=true` 仅显式启用明文开发连接，否则使用 TLS 1.3，并支持 `ANI_STORAGE_GRPC_SERVER_NAME`。Storage client 连接失败会阻止进程启动，未配置时 readiness 保持未就绪。

开发机已通过 runtime、Storage bufconn 协议测试和 `make verify`。外部 Storage 服务端尚未提供，真实跨进程对象存在性、checksum 和下载 URL 过期验证仍为 `not_verified`。
