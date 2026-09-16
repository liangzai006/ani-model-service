# Upload URL handler slice

实现 `GetUploadURL`：

- 从可信 Principal 获取 tenant，拒绝普通字段冒充身份
- 校验 model/version/file name、正数大小和 64 位 SHA256
- 拒绝文件名路径分隔符与换行，避免对象引用穿越
- 通过 Storage port 请求上传地址，返回确定的 tenant-scoped storage path
- 校验 provider 返回地址非空且未过期

版本/任务持久化和幂等键仍待后续事务切片；真实 Storage provider 未接入。

验证：`go test ./internal/service` 通过。
