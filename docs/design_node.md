灵犀AI OS 核心优化 Skill 文档（Everything is Node）

⚠️ 核心约束（Claude必须严格遵守）

本Skill文档用于指导灵犀AI OS优化，核心遵循“Everything is Node” 设计哲学，类比Linux“Everything is File”，系统中所有交互、操作、资源管理均通过Node实现，禁止任何模块间直接交互（必须通过Node中转）。所有优化需完全兼容现有系统设计（DDD分层、数据库结构、调度逻辑），不修改核心类、数据库表结构，仅扩展Node能力与交互规范。

关键补充约束：Node之间支持依赖关系，部分Node需依赖其他Node执行完成（状态为SUCCESS）后，方可进入READY状态并被调度执行；依赖关系通过ai_node_dependency表维护，与现有DAG设计完全兼容，所有依赖相关逻辑均通过Node和现有调度体系实现，不新增独立依赖管理模块。

一、核心哲学定义（必须优先遵循）

1.  系统中 所有可执行单元、交互载体、资源管理入口，均抽象为Node，无例外；

2.  模块间交互（Orchestrator ↔ Worker、Orchestrator ↔ 上下文管理、Worker ↔ 工具/LLM），必须通过Node的输入/输出完成，禁止模块直连；

3.  Node是系统的“最小交互单元+最小执行单元”，兼具“载体”与“执行”双重属性，可承载工具调用、LLM请求、上下文操作、日志输出等所有行为；

4.  Node之间支持依赖关系，子Node需等待所有父Node执行完成（状态为SUCCESS）后，方可被调度执行，依赖关系通过DAG（ai_node_dependency表）维护；

5.  保留现有Node核心结构，仅扩展类型、输入输出协议，确保与现有调度逻辑（数据库DAG、乐观锁、状态机）完全兼容。

二、Node类型扩展（核心优化点）

基于现有NodeType，扩展为6类，覆盖系统所有交互场景，所有类型Node统一遵循现有Node类结构（id、taskId、status等核心字段不变），仅差异化input/output协议；所有类型Node均支持依赖关系，可作为父Node（被依赖）或子Node（依赖其他Node）。

2.1 原有Node类型（保留并完善）

- LLM 类型：处理LLM调用（Prompt输入、生成结果输出），负责与LLM Runtime交互；可被其他Node依赖（如TOOL Node依赖其生成的内容），也可依赖其他Node（如依赖CONTEXT Node获取上下文）；

- TOOL 类型：处理第三方工具调用（API、数据库、文件等），负责工具适配与结果返回；可依赖LLM Node、其他TOOL Node等；

- CONTROL 类型：处理系统控制逻辑（任务暂停/恢复、节点重试触发、调度优先级调整）；可依赖其他Node（如依赖LOG Node记录日志后，触发重试）。

2.2 新增Node类型（实现“Everything is Node”）

- CONTEXT 类型：专门处理上下文管理（快照生成、快照恢复、上下文查询、上下文共享），替代独立上下文管理模块的直接调用；可被LLM/TOOL Node依赖（获取上下文数据），也可依赖其他Node（如依赖LLM Node获取执行结果，保存为上下文）；

- LOG 类型：处理日志输出（基于Node执行状态，自动生成结构化日志、审计日志，无需独立日志模块）；依赖被日志记录的Node（如LLM Node、TOOL Node执行完成后，触发LOG Node生成日志）；

- COMM 类型：处理模块间通信中转（Orchestrator与Worker、Worker与Worker之间的消息传递，基于Redpanda事件，通过Node封装）；可依赖其他Node（如依赖LLM Node执行完成后，生成COMM Node回传结果）。

三、各类型Node详细规范（输入/输出协议+职责+依赖适配）

所有Node的input/output均为JSON格式，严格遵循统一Node JSON协议，新增字段不破坏原有结构，仅在input/output中扩展差异化内容；依赖相关信息通过input传入（关联父Node ID及结果），输出中可携带自身执行结果，供子Node依赖调用。

3.1 LLM 类型 Node

职责：接收LLM调用请求，输出LLM生成结果，与LLM Runtime交互（Orchestrator不直接调用LLM）；支持依赖CONTEXT Node获取上下文，也可作为父Node被TOOL、LOG等Node依赖。

{
  "nodeId": "n1",
  "taskId": "t1",
  "type": "LLM",
  "name": "llm_generate",
  "input": {
    "prompt": "写一篇AI OS相关文章",
    "model": "gpt-4",
    "tokenLimit": 2000,
    "contextId": "ctx-123", // 关联上下文Node的id，用于获取上下文（依赖CONTEXT Node）
    "depsResult": { // 父Node执行结果（若有依赖，由Orchestrator自动注入）
      "n4": { // 依赖的CONTEXT Node id
        "snapshotId": "snap-123",
        "contextData": "..."
      }
    }
  },
  "output": {
    "content": "AI OS是面向自主Agent的操作系统...",
    "tokenUsed": 800,
    "contextId": "ctx-123", // 回写上下文关联id，供子Node依赖
    "depsMet": true // 标记自身执行完成，供子Node判断依赖是否满足
  },
  "status": "SUCCESS",
  "retry": {
    "count": 0,
    "max": 3
  },
  "priority": 5,
  "workerGroup": "llm-group"
}

3.2 TOOL 类型 Node

职责：接收工具调用参数，调用第三方工具，返回工具执行结果，与工具适配器交互；通常依赖LLM Node（获取输入参数），也可依赖其他TOOL Node。

{
  "nodeId": "n2",
  "taskId": "t1",
  "type": "TOOL",
  "name": "http_call",
  "input": {
    "url": "https://api.example.com/data",
    "method": "POST",
    "headers": {"Content-Type": "application/json"},
    "body": {"param1": "value1"},
    "timeout": 5000,
    "depsResult": { // 依赖的父Node执行结果（此处依赖LLM Node n1）
      "n1": {
        "content": "AI OS是面向自主Agent的操作系统...",
        "tokenUsed": 800
      }
    }
  },
  "output": {
    "statusCode": 200,
    "responseBody": {"result": "success"},
    "executionTime": 1200,
    "depsMet": true
  },
  "status": "SUCCESS",
  "retry": {
    "count": 0,
    "max": 3
  },
  "priority": 4,
  "workerGroup": "tool-group"
}

3.3 CONTROL 类型 Node

职责：处理系统控制逻辑，接收控制指令，触发对应系统操作（由Orchestrator调度执行）；可依赖LOG Node（确认日志记录完成后，触发重试）、其他Node（确认父Node执行状态）。

{
  "nodeId": "n3",
  "taskId": "t1",
  "type": "CONTROL",
  "name": "node_retry",
  "input": {
    "targetNodeId": "n1", // 要重试的节点id
    "retryReason": "LLM调用超时",
    "retryCount": 1,
    "depsResult": { // 依赖的父Node执行结果（此处依赖LOG Node n5）
      "n5": {
        "logId": "log-123",
        "logContent": "[ERROR] 节点n1（LLM类型）执行失败，原因：超时"
      }
    }
  },
  "output": {
    "targetNodeStatus": "READY",
    "retryResult": "success",
    "nextScheduleTime": "2024-08-01T10:05:00",
    "depsMet": true
  },
  "status": "SUCCESS",
  "retry": {
    "count": 0,
    "max": 1
  },
  "priority": 6,
  "workerGroup": "control-group"
}

3.4 CONTEXT 类型 Node（核心新增）

职责：处理上下文快照、恢复、查询、共享，替代独立上下文管理模块，所有上下文操作均通过该类型Node完成；可被LLM/TOOL Node依赖，也可依赖其他Node（获取需保存的上下文数据）。

{
  "nodeId": "n4",
  "taskId": "t1",
  "type": "CONTEXT",
  "name": "context_snapshot",
  "input": {
    "operation": "save", // save/restore/query/share
    "contextData": {
      "nodeId": "n1",
      "input": "...",
      "output": "...",
      "depsStatus": {"n1": "SUCCESS"},
      "timestamp": "2024-08-01T10:00:00"
    },
    "snapshotId": "snap-123",
    "shareWith": ["n2", "n5"], // 可共享给其他节点的id（子Node依赖）
    "depsResult": { // 依赖的父Node执行结果（此处依赖LLM Node n1）
      "n1": {
        "content": "AI OS是面向自主Agent的操作系统...",
        "status": "SUCCESS"
      }
    }
  },
  "output": {
    "operationResult": "success",
    "snapshotId": "snap-123",
    "storageLocation": "redis+pg", // 热数据redis，冷数据pg
    "expireTime": "2024-08-08T10:00:00",
    "depsMet": true
  },
  "status": "SUCCESS",
  "retry": {
    "count": 0,
    "max": 2
  },
  "priority": 5,
  "workerGroup": "context-group"
}

3.5 LOG 类型 Node（核心新增）

职责：自动生成结构化日志、审计日志，无需独立日志模块，日志内容从Node自身状态和输入输出提取；必须依赖被日志记录的Node（父Node执行完成后，方可生成日志）。

{
  "nodeId": "n5",
  "taskId": "t1",
  "type": "LOG",
  "name": "execution_log",
  "input": {
    "relatedNodeId": "n1", // 关联的执行节点id（父Node，必须依赖）
    "logType": "execution", // execution/audit/error
    "logLevel": "INFO",
    "depsResult": { // 依赖的父Node（n1）执行结果，用于生成日志内容
      "n1": {
        "status": "SUCCESS",
        "executionTime": 800,
        "tokenUsed": 800
      }
    }
  },
  "output": {
    "logId": "log-123",
    "logContent": "[INFO] 节点n1（LLM类型）执行成功，耗时800ms，token使用800",
    "timestamp": "2024-08-01T10:01:00",
    "traceId": "trace-abc",
    "storagePath": "loki://aios/logs/execution",
    "depsMet": true
  },
  "status": "SUCCESS",
  "retry": {
    "count": 0,
    "max": 1
  },
  "priority": 3,
  "workerGroup": "log-group"
}

3.6 COMM 类型 Node（核心新增）

职责：模块间通信中转，Orchestrator与Worker、Worker与Worker之间的消息，均通过该类型Node封装后发送到Redpanda；可依赖其他Node（父Node执行完成后，传递结果消息）。

{
  "nodeId": "n6",
  "taskId": "t1",
  "type": "COMM",
  "name": "message_transfer",
  "input": {
    "sender": "orchestrator", // 发送方模块
    "receiver": "worker-group-1", // 接收方模块/Worker组
    "messageType": "node_ready", // 消息类型（与Redpanda Topic对应）
    "messageContent": {
      "nodeId": "n1",
      "taskId": "t1",
      "status": "READY"
    },
    "topic": "ai.node.ready", // Redpanda Topic
    "depsResult": { // 依赖的父Node执行结果（如依赖CONTEXT Node完成快照）
      "n4": {
        "operationResult": "success",
        "snapshotId": "snap-123"
      }
    }
  },
  "output": {
    "sendResult": "success",
    "messageId": "msg-123",
    "topic": "ai.node.ready",
    "sendTime": "2024-08-01T10:02:00",
    "depsMet": true
  },
  "status": "SUCCESS",
  "retry": {
    "count": 0,
    "max": 3
  },
  "priority": 4,
  "workerGroup": "comm-group"
}

四、Node依赖关系核心规范（新增重点）

Node之间的依赖关系完全遵循现有DAG设计，通过ai_node_dependency表（parent_node_id、child_node_id）维护，所有依赖逻辑均集成到现有调度体系，不新增独立模块，核心规范如下：

1. 依赖定义：父Node（parent_node_id）是被依赖的Node，子Node（child_node_id）是依赖其他Node的Node；一个子Node可依赖多个父Node，一个父Node可被多个子Node依赖；

2. 就绪条件：子Node必须满足 所有父Node状态均为SUCCESS，且自身状态为CREATED，方可被Scheduler识别为“可执行节点”（对应原有SQL调度逻辑）；

3. 依赖传递：若父Node自身存在依赖（祖父Node），则需祖父Node执行完成后，父Node方可执行，子Node需等待所有层级的父Node均完成；

4. 失败处理：若任意父Node执行失败（状态为FAILED），则子Node状态自动置为FAILED（或等待父Node重试成功），不进入调度队列；父Node重试成功后，子Node重新判断依赖是否满足，符合条件则进入READY状态；

5. 结果传递：父Node的执行结果（output），由Orchestrator自动注入到子Node的input.depsResult中，供子Node使用（无需子Node主动查询）。

五、模块间交互规范（基于Node实现，核心优化）

所有模块（Orchestrator、Worker、Context、Tool、LLM Runtime）之间的交互，必须通过对应类型Node完成，禁止任何直接调用；结合Node依赖关系，严格遵循以下流程：

5.1 Orchestrator ↔ Worker 交互（含依赖处理）

1. Orchestrator 通过SQL查询“所有父Node均为SUCCESS、自身状态为CREATED”的可执行Node；

2. Orchestrator 生成 COMM 类型 Node，封装节点就绪消息（含子Node依赖的父Node结果），发送到Redpanda对应Topic；

3. Worker 消费Redpanda消息，解析出 COMM Node 的 messageContent，获取待执行Node信息及依赖的父Node结果；

4. Worker 执行完Node（LLM/TOOL类型）后，生成新的 COMM 类型 Node，封装执行结果，发送到Redpanda；

5. Orchestrator 消费结果消息，解析 COMM Node，触发 StateMachine 更新该Node状态为SUCCESS/FAILED；

6. Orchestrator 自动检查该Node的所有子Node，判断子Node的所有父Node是否均为SUCCESS，若满足则将子Node状态置为READY，进入下一轮调度。

5.2 Orchestrator ↔ 上下文管理交互（含依赖处理）

1. 父Node（如LLM Node）执行完成后，Orchestrator 生成 CONTEXT 类型 Node（依赖该LLM Node），指定操作类型（save）和上下文数据（父Node的input/output）；

2. CONTEXT Node 执行时，读取input中的depsResult（父Node结果），完成上下文快照的保存，输出操作结果；

3. Orchestrator 通过查询 CONTEXT Node 的 output，确认快照保存成功，随后检查依赖该CONTEXT Node的子Node（如其他LLM Node），满足依赖条件则调度子Node；

4. 当需要恢复上下文时，Orchestrator 生成 CONTEXT 类型 Node（operation=restore），依赖失败Node的历史快照Node，完成状态恢复后，触发CONTROL Node重试失败Node。

5.3 Worker ↔ Tool/LLM 交互（含依赖处理）

1. Worker 接收 COMM Node 消息后，根据待执行Node的类型（LLM/TOOL），解析其input中的depsResult（父Node结果）；

2. Worker 将父Node结果注入到LLM/TOOL的调用参数中，执行 LLM/TOOL Node；

3. Worker 读取 LLM/TOOL Node 的 output，封装到 COMM Node 中，回传给 Orchestrator，供Orchestrator更新子Node的依赖状态。

5.4 日志生成交互（含依赖处理）

1. 任何Node（父Node）执行状态变更（RUNNING → SUCCESS/FAILED）时，Orchestrator 自动生成 LOG 类型 Node，将该父Node设为依赖节点；

2. LOG Node 读取input中的depsResult（父Node的状态、输入输出、耗时等信息），生成标准化日志；

3. LOG Node 执行完成后，将日志输出到指定存储（Loki/PG），Orchestrator 检查依赖该LOG Node的子Node（如CONTROL Node），满足条件则调度子Node。

六、数据库设计适配（不修改原有结构，仅扩展）

基于现有ai_node、ai_task、ai_node_dependency三张表，无需新增表，仅对ai_node表做微小扩展，确保兼容性，同时完善依赖关系的存储与查询：

1. ai_node表的type字段，新增3个值：CONTEXT、LOG、COMM，与扩展后的NodeType对应；

2. ai_node表的input、output字段（JSON类型），自动兼容新增Node类型的输入输出协议，新增depsResult相关字段，无需修改字段结构；

3. ai_node_dependency表完全复用，用于维护Node之间的依赖关系（parent_node_id、child_node_id），无需修改表结构；

4. 调度逻辑（找可执行节点SQL、乐观锁抢占SQL）完全不变，该SQL已天然支持依赖关系判断（子Node需所有父Node为SUCCESS），新增Node类型与原有类型统一参与调度。

补充：可执行节点查询SQL（原有逻辑，已适配依赖关系）

SELECT n.*
FROM ai_node n
WHERE n.status = 'CREATED'
AND NOT EXISTS (
    SELECT 1
    FROM ai_node_dependency d
    JOIN ai_node p ON d.parent_node_id = p.id
    WHERE d.child_node_id = n.id
    AND p.status != 'SUCCESS'
);

七、核心优化目标（Claude生成代码时必须达成）

- 1.  实现“Everything is Node”：系统中所有交互、操作、资源管理均通过Node完成，删除所有模块间直接调用代码；

- 2.  完善Node依赖关系：实现Node间依赖的定义、就绪判断、结果传递、失败处理，完全适配现有DAG调度逻辑；

- 3.  兼容性：所有优化不破坏现有系统设计，与原有DDD分层、调度逻辑、数据库结构完全兼容；

- 4.  可扩展性：新增Node类型可灵活扩展，后续新增功能（如监控、告警）可直接新增对应Node类型，无需修改核心代码；

- 5.  可观测性：通过LOG Node实现全链路日志统一，通过CONTEXT Node实现全链路状态可恢复、可回溯；

- 6.  解耦性：Orchestrator、Worker、Tool、LLM Runtime完全解耦，仅通过Node和Redpanda事件交互，依赖关系通过DAG统一管理。

八、测试用例（Claude生成代码后必须执行）

所有测试用例需验证“Everything is Node”原则和Node依赖关系，确保模块间无直接交互，所有依赖逻辑正常生效：

8.1 测试用例1：串行依赖任务（LLM → CONTEXT → TOOL → LOG）

流程：创建Task → 生成LLM Node（n1，写文章）→ 生成CONTEXT Node（n4，依赖n1，保存快照）→ 生成TOOL Node（n2，依赖n4，总结文章）→ 生成LOG Node（n5，依赖n2，记录日志）→ 所有Node执行完成，Task状态为SUCCESS；

验证点：n4需等待n1成功后执行，n2需等待n4成功后执行，n5需等待n2成功后执行；依赖结果正常传递，无直接调用。

8.2 测试用例2：并行依赖任务（n1 → n3；n2 → n3）

流程：创建Task → 生成LLM Node（n1，写文章）、TOOL Node（n2，获取数据）→ 生成COMM Node（n3，依赖n1和n2，传递合并结果）→ n1、n2并行执行，均成功后n3执行；

验证点：n3需等待n1和n2均为SUCCESS后，方可进入READY状态；并行执行正常，依赖判断准确。

8.3 测试用例3：上下文快照与恢复（含依赖）

流程：LLM Node（n1）执行失败 → 生成LOG Node（n5，依赖n1，记录错误日志）→ 生成CONTROL Node（n3，依赖n5，触发重试）→ 生成CONTEXT Node（n4，依赖n3，恢复n1快照）→ n1重试成功；

验证点：依赖链条（n1→n5→n3→n4→n1）正常生效，上下文恢复通过CONTEXT Node完成，依赖结果正常传递。

8.4 测试用例4：父Node失败导致子Node失败

流程：LLM Node（n1）执行失败 → TOOL Node（n2，依赖n1）→ Orchestrator自动将n2状态置为FAILED；n1重试失败后，Task状态置为FAILED；

验证点：子Node（n2）因父Node（n1）失败无法执行，状态正确更新，依赖失败处理逻辑生效。

九、给Claude的最终指令

请严格遵循本Skill文档，基于“Everything is Node”原则和Node依赖关系，对灵犀AI OS进行优化，生成完整可运行的代码（Java + Spring Boot），要求：

1. 扩展NodeType枚举，新增CONTEXT、LOG、COMM三种类型，完善所有Node类型的输入输出协议，新增depsResult相关字段，支持依赖结果传递；

2. 完善OrchestratorService、Scheduler、StateMachine等核心类，新增Node依赖关系的判断、结果注入、状态联动逻辑，确保子Node需等待父Node成功后再调度；

3. 实现CONTEXT、LOG、COMM三种新增Node的执行逻辑，适配依赖关系，确保依赖的父Node结果可正常读取和使用；

4. 确保代码与现有数据库结构、Redpanda配置、DDD分层完全兼容，无需修改数据库表结构，复用ai_node_dependency表维护依赖关系；

5. 生成后自动执行所有测试用例，验证“Everything is Node”原则和Node依赖关系落地，确保所有操作均通过Node完成、依赖逻辑正常生效；

6. 所有代码有清晰注释，符合生产级规范，可直接集成到现有项目中。