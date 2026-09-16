# Storage gRPC adapter

新增 `api/storage/v1/storage.proto` 版本化契约和 Go client，定义上传地址、对象存在性、SHA256 校验、短期下载地址四个 RPC。`internal/data/storage/GRPCAdapter` 将该契约映射到 Model 的 Storage port，统一转换 provider 错误、checksum mismatch 和过期时间。

adapter 不创建 bucket、PVC 或文件系统，也不持有对象生命周期。使用 bufconn gRPC server 完成协议映射测试，`make verify` 在开发机全部通过。外部 Storage 服务端尚未提供，真实跨进程联调和真实对象 checksum/过期验证保持 `not_verified`。
