# Storage gRPC 失败语义验证

补充 Storage gRPC adapter 的 bufconn 测试：`matches=false` 映射为 `ErrChecksumMismatch`；过期时间由 adapter 原样返回并由 Model Storage port 的 `ValidateSignedURL` 拒绝；对象存在性响应保持 false，不被转换为成功对象。开发机 `make verify` 全部通过。

这些是协议和错误映射证据，不代表外部 Storage 服务端已联调；真实对象和下载地址验证仍为 `not_verified`。
