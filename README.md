# protobuf-wire-inspector

离线协议排查用的受限 Protobuf 解码工具。仅使用 Go 标准库，不依赖生成代码或第三方协议库。

## 支持范围

- 标量类型：`uint64`、`sint64`（ZigZag）、`fixed32`（四字节小端）
- 长度限定类型：`string`（必须合法 UTF-8）、`bytes`（输出十六进制）
- 嵌套消息：通过内联 `message` 描述递归解码
- 可重复整数（`"repeated": true`）：同时接受逐项与打包（packed）编码，逐元素输出以保持出现顺序
- 未知字段：按线类型跳过，记录编号与原始十六进制
- 不支持：group 线类型、`.proto` 文件解析、fixed64/double/float/枚举等未列出类型

## 字段描述（JSON）

```json
{
  "fields": [
    {"number": 1, "name": "id", "type": "uint64"},
    {"number": 2, "name": "delta", "type": "sint64"},
    {"number": 3, "name": "vals", "type": "uint64", "repeated": true},
    {"number": 4, "name": "label", "type": "string"},
    {"number": 5, "name": "sub", "type": "message", "message": {
      "fields": [{"number": 1, "name": "inner", "type": "uint64"}]
    }}
  ]
}
```

编号（1..2^29-1）与名称在同一消息内均不可重复。

## 命令行

```sh
# 单条报文
go run ./cmd/pbinspect -schema schema.json -hex "089601"

# 批量 JSONL（每行一条十六进制字符串或 {"hex": "..."}）
go run ./cmd/pbinspect -schema schema.json -batch input.jsonl
```

## 输出格式

每条输入输出一行 JSON。成功时 `records` 按字段出现顺序逐条记录（不合并重复单值字段，不补缺省值）：

```json
{"line":1,"ok":true,"records":[
  {"path":"id","number":1,"name":"id","type":"uint64","offset":0,"value":150},
  {"path":"field_9","number":9,"offset":3,"unknown":true,"wire":0,"hex":"2a"}
]}
```

- `offset` 为相对整条报文起点的字节偏移（标签所在位置）
- 嵌套消息的 `value` 为嵌套的 records 数组，`path` 以 `.` 连接
- 未知字段带 `unknown: true`、`wire` 与原始值 `hex`

失败时输出 `error`，错误信息包含字节偏移；批量模式下单条失败不阻断其他报文，末尾输出汇总：

```json
{"line":2,"ok":false,"error":"offset 1: truncated varint"}
{"summary":{"total":3,"ok":2,"failed":1}}
```

## 严格性

- 拒绝零字段号、超过 29 位的字段号、非法线类型（含 group）、变长整数溢出与截断数据
- 长度限定内容必须完整落在所属消息范围内
- 字段线类型与声明不符即拒绝，不猜测类型

## 测试

```sh
go test ./...
```
