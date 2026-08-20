# 部署说明

服务入口固定为 `cmd/server`，读取环境变量：

- `PORT`
- `DB_PATH`

SQLite 使用真实文件持久化，进程关闭后重新以同一个 `DB_PATH` 启动即可恢复数据。
