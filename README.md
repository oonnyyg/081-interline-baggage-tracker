# 联运行李追踪工具（interline-baggage-tracker）

根据计划航段和机场扫描记录，逐行解释每件联运行李当前在哪里、正在乘坐哪个航班、下一步该做什么。仅使用 Go 标准库，不连接任何真实航班系统。

## 运行

```sh
go run ./cmd/baggage --state ./state
```

- `--state`：状态目录，不存在会自动创建；其中 `journal.jsonl` 为追加式事件日志。
- 标准输入每行一个 JSON 请求，标准输出每行一个 JSON 响应。
- 成功的写操作按顺序追加到日志并 `fsync`；退出重开时重放日志，状态和事件编号完全恢复。
- 非法请求返回 `{"ok":false,"error":...}`，不更新位置、不消耗扫描事件编号、不写日志，后续行继续处理。查询（`query`）不写日志。

## 请求协议

所有字段名均为小写。时间为整数（同一行李的扫描时间必须严格递增；首次扫描时间不限）。

### 登记航班 `flight`

```json
{"op":"flight","id":"CA101","from":"PEK","to":"CAN"}
```

- `id` 全局唯一；`from` 与 `to` 必须不同且非空。

### 登记行李 `bag`

```json
{"op":"bag","label":"B1","segments":["CA101","CA202"]}
```

- `label` 唯一；`segments` 为非空航班编号序列。
- 相邻航段必须首尾衔接（前段 `to` == 后段 `from`），航班不得重复，航班必须已登记。
- 行李初始位置为首段出发机场（地面）。

### 扫描事件 `scan`

```json
{"op":"scan","event":"e1","label":"B1","type":"load","flight":"CA101","time":1}
```

- `event` 为全局唯一事件编号，用于去重。
- `type` 取值：
  - `load`：装机。只允许在当前机场装入“剩余计划的第一个航班”，必须带匹配的 `flight`。
  - `unload`：落地卸机。只允许卸下当前已装的那个航班；行李移动到该航班到达机场，该航段进入 `completed`，计划推进一段。`flight` 必填且必须是当前所乘航班。
  - `remove`：装机后撤下。在出发机场把已装行李撤下，行李回到该航班的出发机场，**计划不推进**；之后可以继续装该航段或改签。`flight` 必填且必须是当前所乘航班。
  - `delivered`：到达最终机场（剩余计划为空、在地面）后交付给旅客；不带 `flight`。交付后禁止一切扫描和改签。
- 已完成的航段不能再次装入。
- 同件行李时间必须严格递增。
- 去重规则：
  - 完全相同的事件（同一 `event` 且整个请求 JSON 内容一致）重复提交 → 返回第一次的原结果，不重复改状态、不新增扫描。
  - 同一 `event` 但内容不同 → 拒绝（`ok:false`），且该编号的原结果仍然有效。
  - 被拒绝的新扫描不消耗其事件编号，可修正后用同编号重提。

### 改签 `rebook`

```json
{"op":"rebook","label":"B1","segments":["CA303"]}
```

- 仅地面行李可改签（已装机必须先 `unload` 或 `remove`），已交付行李不可改签。
- 新计划非空、航班不重复（也不得使用已完成航班），首段必须从当前位置出发，后续依次连通。
- 已完成航段历史保持不变，`remaining`/`plan` 更新为新计划。

### 查询 `query`

```json
{"op":"query","label":"B1"}
```

返回当前状态（`ground`+`location`、`loaded`+`flight` 或 `delivered`+`location`）、完整 `plan`、`completed`、`remaining`，以及按时间升序排列的 `scans`。

## 典型会话

中转：

```json lines
{"op":"flight","id":"CA101","from":"PEK","to":"CAN"}
{"op":"flight","id":"CA202","from":"CAN","to":"SIN"}
{"op":"bag","label":"B1","segments":["CA101","CA202"]}
{"op":"scan","event":"e1","label":"B1","type":"load","flight":"CA101","time":1}
{"op":"scan","event":"e2","label":"B1","type":"unload","flight":"CA101","time":2}
{"op":"scan","event":"e3","label":"B1","type":"load","flight":"CA202","time":3}
{"op":"scan","event":"e4","label":"B1","type":"unload","flight":"CA202","time":4}
{"op":"scan","event":"e5","label":"B1","type":"delivered","time":5}
```

装机后撤下并重新托运（改签）：

```json lines
{"op":"flight","id":"CA303","from":"CAN","to":"BKK"}
{"op":"scan","event":"e6","label":"B1","type":"load","flight":"CA202","time":6}
{"op":"scan","event":"e7","label":"B1","type":"remove","flight":"CA202","time":7}
{"op":"rebook","label":"B1","segments":["CA303"]}
```

注：上面两段示例不能直接连跑（B1 已交付，仅为分别演示场景）。

## 测试

```sh
go test ./...
```

覆盖：航班/行李校验、中转全流程、错误顺序扫描、装机撤下、卸下/撤下后改签、事件编号去重与冲突、错误不消耗编号、日志重放恢复、CLI 逐行 I/O 与错误隔离。
