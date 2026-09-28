# Win Admin

Win Admin 是一个基于 Wails v3 的 Windows 系统管理工具，当前包含资源管理器上下文菜单、打开方式、Windows 计划任务、环境变量和应用卸载管理。

## 技术栈

- Go 1.25+ / Wails v3.0.0-beta.16
- React 19 + TypeScript（ES2023）+ Vite
- Bun
- Tailwind CSS v4
- shadcn/ui Base UI + Nova 默认风格

## 当前功能

上下文菜单支持四个 Explorer 目标：

- 文件：`HKLM\Software\Classes\*\shell`
- 目录：`HKLM\Software\Classes\Directory\shell`
- 目录背景：`HKLM\Software\Classes\Directory\Background\shell`
- 桌面背景：`HKLM\Software\Classes\DesktopBackground\shell`

支持：

- 查看已有静态 Shell Verb
- 新增、编辑、删除、启用或禁用菜单
- 普通命令与递归多级菜单
- 直接使用 Windows 原生目标占位符
- 文件和目录使用 `%1`
- 目录背景和桌面背景使用 `%V`
- 在 win-admin 管理的菜单命令中使用 `%u`，将目标路径转换为 `file:///` 地址；包含 `%u` 的命令会通过非提权启动器执行
- 从命令推断图标来源
- 选择 EXE、DLL 或 ICO 作为图标来源
- `Position=Top` / `Bottom`
- 修改后调用 `SHChangeNotify` 通知 Explorer 刷新关联缓存

不尝试编辑无法安全还原的 COM / CommandStore 动态扩展，这类条目在界面中标记为只读。

打开方式支持按扩展名注册自定义命令，例如 `ts` 使用 `bun run`，`html` 使用 `msedge --app="%u"`。`%1` 表示原始文件路径，`%u` 表示经过 URL 编码的 `file:///` 地址。两者都省略时，文件路径会作为命令的最后一个参数传入。

保存后需在 Windows 的“默认应用”设置中将对应扩展名设为“win-admin”；应用不会直接修改系统维护的用户默认选择。修改扩展名或删除已设为默认的命令后，旧扩展名可能需要重新选择默认应用。

计划任务支持：

- 递归查看 Task Scheduler 中的任务
- 搜索任务；任务右键菜单支持运行、停止、启用、禁用、编辑和删除
- 新建任务与安全的结构化编辑
- 从 Windows Task Scheduler XML 导入并自动填充新建编辑器
- 将任意已注册任务按原始 Task Scheduler XML 导出
- 单个程序操作（Exec Action）
- 手动、一次、每天、每周、登录时和系统启动时触发
- 普通权限或最高权限运行
- 计划任务模板：保存为模板、从模板新建、编辑、重命名和删除
- 对多操作、多触发器、密码凭据或未支持触发器的复杂任务保持只读编辑，避免破坏原定义

环境变量支持：

- 查看和过滤系统、用户环境变量
- 新增、编辑、删除环境变量
- 普通环境变量使用宽版多行值编辑器
- PATH 专用列表编辑、排序和删除
- 展开 `%VAR%` 后检查 PATH 项，并标记不存在的路径
- 修改后广播 `WM_SETTINGCHANGE` 通知 Windows 环境变化

应用卸载支持：

- 读取当前用户和系统、32/64 位卸载注册表项
- 按名称、版本和发布者过滤
- 表格展示名称、发布者、版本、安装时间和注册表估算大小
- 点击名称、安装时间或大小表头排序，再次点击切换升降序
- 通过卸载按钮或右键菜单运行应用注册的原生卸载程序
- 右键打开安装位置、定位卸载注册表项或复制注册表路径
- 卸载命令只在后端重新读取和执行，不暴露给前端

## 管理员权限

Windows manifest 使用 `requireAdministrator`。正式构建启动时会触发 UAC，因为上下文菜单默认写入 HKLM 并对所有用户生效。

开发模式需要从已经提升权限的终端运行，否则界面可以查看，但写操作会被后端拒绝。

## 开发

安装前端依赖：

```powershell
cd frontend
bun install
```

启动 Wails 开发模式：

```powershell
cd ..
task dev
```

启动带 WebView2 CDP 调试端口的构建：

```powershell
task debug:webview
```

默认端口为 `9222`，可通过 `WIN_ADMIN_WEBVIEW_DEBUG_PORT` 覆盖。

生成绑定：

```powershell
task common:generate:bindings
```

## 构建与验证

运行 Go 测试：

```powershell
go test ./...
```

前端生产构建：

```powershell
cd frontend
bun run build
```

Windows 完整构建：

```powershell
cd ..
task build
```

产物默认位于：

```text
bin\win-admin.exe
```

## 模块边界

上下文菜单由 `ContextMenuService` 和 `frontend/src/features/context-menu` 管理；计划任务由 `ScheduledTaskService`、`ScheduledTaskTemplateService` 和 `frontend/src/features/scheduled-tasks` 管理；环境变量由 `EnvironmentVariableService` 和 `frontend/src/features/environment-variables` 管理；应用卸载由 `AppUninstallService` 和 `frontend/src/features/app-uninstall` 管理。

