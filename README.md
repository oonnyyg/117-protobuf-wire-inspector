# Protobuf Wire Inspector

离线协议排查工具：仅使用 Go 标准库，按 JSON 字段描述把十六进制
Protobuf 报文解码成可读的逐字段记录。不依赖生成代码，不解析 `.proto`
文件，不使用第三方协议库。

## 支持范围

- 字段类型：`uint64`、`sint64`（ZigZag）、`fixed32`（四字节小端）、
  `string`（必须合法 UTF-8）、`bytes`（输出十六进制）、`message`
  （内联描述的嵌套消息，递归解码）。
- 可重复整数字段（`"repeated": true`）同时接受逐项编码与打包
  （packed）编码，展开为每条记录并保持实际出现顺序。
- 未知字段按线类型跳过，记录编号、线类型与原始十六进制，不影响
  后续已知字段。
- 不支持：group 类型、未列出的其他类型、`.proto` 解析、类型猜测
  （字段线类型与声明不符时直接拒绝该报文）。

## 校验与错误

- 拒绝零字段号、超过 29 位的字段号、非法线类型、varint 溢出
  （超过 64 位）与截断数据。
- 长度限定内容必须完整落在所属消息范围内。
- 所有错误带相对整条报文起点的字节偏移。

## 字段描述格式

```json
{
  "fields": [
    {"number": 1, "name": "id", "type": "uint64"},
    {"number": 2, "name": "delta", "type": "sint64"},
    {"number": 6, "name": "samples", "type": "uint64", "repeated": true},
    {"number": 7, "name": "child", "type": "message", "message": {
      "fields": [
        {"number": 1, "name": "note", "type": "string"}
      ]
    }}
  ]
}
```

编号与名称在同一消息内不可重复；`message` 类型必须带内联的
`message` 描述。

## 输入与输出

输入为 JSONL，每行一条报文：

```json
{"hex": "0896011003"}
```

每行输出一个 JSON 对象，一条报文失败不阻断其他报文：

```json
{"line":1,"ok":true,"records":[{"offset":0,"field":1,"name":"id","type":"uint64","value":150}]}
{"line":2,"ok":false,"error":"offset 1: truncated varint","offset":1}
```

记录字段：

- `offset` / `field` / `name` / `type` / `value`：已知字段的出现记录；
  嵌套消息的 `value` 是记录数组；`bytes` 的 `value` 是十六进制字符串。
- `unknown` / `wire_type` / `hex`：未知字段的编号、线类型与原始字节。

按每次字段出现输出记录：不合并重复的单值字段，不补缺省值。
批处理结束后在 stderr 输出汇总：`summary: total=N ok=N failed=N`。

## 使用

```sh
go run . -schema schema.json -input messages.jsonl > out.jsonl
# 或从标准输入读取
cat messages.jsonl | go run . -schema schema.json
```

## 测试

```sh
go test ./...
```

覆盖负数 ZigZag、逐项与打包混合编码、嵌套消息截断偏移、未知字段
跳过、线类型不匹配、非法 UTF-8、varint 溢出与描述校验。