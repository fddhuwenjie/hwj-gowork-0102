# HTTP JSON 接口

统一错误格式：

```json
{"error":{"code":"invalid_state","message":"非法状态转换","details":null}}
```

主要端点：

- `GET /healthz`
- `POST /api/v1/welds`
- `GET /api/v1/welds`
- `GET /api/v1/welds/{id}`
- `PUT /api/v1/welds/{id}`
- `POST /api/v1/welds/{id}/transition`
- `DELETE /api/v1/welds/{id}`
- `POST /api/v1/welds/batch`
- `POST /api/v1/operations/schedule-inspection`
- `POST /api/v1/operations/submit-report`
- `POST /api/v1/operations/create-repair`
- `POST /api/v1/operations/approve-review`
- `GET /api/v1/queries/overdue-reinspections`
- `GET /api/v1/queries/expired-calibration-reports`

分页参数：`limit`、`offset`、`sort`、`q`。
