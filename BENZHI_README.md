# AuditLog

AuditLog 是一个自包含的不可变审计日志留存服务。日志以块为单位追加写入，
块之间通过哈希链串联，写入后不可改写；服务提供按时间与关键字检索、分页、
归档、导出、保留策略清理与全链完整性校验能力。

## 构建

```bash
go build -mod=vendor ./...
```

## 运行

```bash
go run -mod=vendor ./cmd/auditlog -addr 127.0.0.1:8090 -dir ./data
```

启动后访问 http://127.0.0.1:8090/ 打开控制台页面。

## HTTP 接口

- `POST /api/v1/records` 追加一条审计记录
- `GET /api/v1/records` 按条件检索并分页
- `GET /api/v1/records/{seq}` 按序号读取单条记录
- `POST /api/v1/verify` 执行全链完整性校验
- `POST /api/v1/archive` 按窗口归档日志块
- `POST /api/v1/retention` 按保留策略清理过期块
- `POST /api/v1/export` 启动导出并返回游标
- `GET /api/v1/export/{id}` 按游标续传导出
- `GET /healthz` 健康检查

## 存储布局

数据目录下包含 `blocks/`、`archive/`、`journal/`、`meta/`、`cursor/`
与 `index.snap`，块文件为不可变追加文件，归档与清理会移动或删除块文件。
