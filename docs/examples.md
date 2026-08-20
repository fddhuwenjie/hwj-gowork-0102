# 示例

```bash
curl -s http://localhost:8080/healthz
```

查询超过指定天数未复探焊缝：

```bash
curl 'http://localhost:8080/api/v1/queries/overdue-reinspections?days=7'
```
