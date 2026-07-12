# Electron 本地历史目录统一设计

## 目标

在开发环境中，通过 Electron 启动和通过 `scripts/start-local-backend.sh` 启动的本地 agent 使用相同的数据目录，使重启或切换启动入口后仍能显示既有标书项目历史。

## 范围

- Electron 开发启动默认使用仓库的 `local-backend/data`。
- 若设置 `TANGYING_LOCAL_DATA_DIR`，Electron 优先使用该显式目录。
- 打包后的 Electron 应用继续使用 `app.getPath('userData')/local-agent`。
- 为目录解析逻辑增加自动化回归测试。

## 不在范围内

- 不移动、删除或重写既有项目、产物和历史快照。
- 不改变 `scripts/start-local-backend.sh` 的默认目录。
- 不对已发布桌面版本执行跨目录数据迁移。

## 设计

将 Electron 本地 agent 的数据目录解析提取为一个无 Electron 依赖的函数。解析优先级为：

1. `TANGYING_LOCAL_DATA_DIR`；
2. 开发模式下仓库根目录的 `local-backend/data`；
3. 打包模式下 Electron 用户数据目录的 `local-agent` 子目录。

`main.cjs` 仅调用该函数并将结果传递给子进程环境变量。目录解析函数用 Node 内置测试框架覆盖三种优先级，以防后续修改重新造成启动入口分裂。

## 验收标准

- `npm run electron:dev` 启动的本地 agent 默认使用 `local-backend/data`。
- 显式设置 `TANGYING_LOCAL_DATA_DIR` 时，该路径不被覆盖。
- 打包应用仍在用户数据目录保存本地数据。
- 目录解析回归测试通过，前端构建通过。
