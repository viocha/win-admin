import { useCallback, useEffect, useMemo, useRef, useState } from "react"
import {
  ChevronRightIcon,
  CopyPlusIcon,
  FileDownIcon,
  FolderIcon,
  FolderOpenIcon,
  FolderPlusIcon,
  HistoryIcon,
  LoaderCircleIcon,
  PencilIcon,
  PlayIcon,
  PlusIcon,
  PowerIcon,
  RefreshCwIcon,
  SearchIcon,
  SquareIcon,
  Trash2Icon,
} from "lucide-react"

import {
  ScheduledTaskService,
  ScheduledTaskTemplateService,
  type ScheduledTaskEdit,
  type ScheduledTaskHistoryEvent,
  type ScheduledTaskSummary,
  type ScheduledTaskTemplate,
} from "../../../bindings/win-admin"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuTrigger,
} from "@/components/ui/context-menu"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import {
  Empty,
  EmptyContent,
  EmptyHeader,
  EmptyTitle,
} from "@/components/ui/empty"
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from "@/components/ui/input-group"
import { ScrollArea } from "@/components/ui/scroll-area"
import { SidebarTrigger } from "@/components/ui/sidebar"
import { Switch } from "@/components/ui/switch"
import { Table, TableBody, TableCell, TableHead, TableHeader, TableRow } from "@/components/ui/table"
import { TruncatedText } from "@/components/ui/truncated-text"
import { showError, showErrorMessage } from "@/lib/error-toast"
import { ScheduledTaskEditorDialog } from "./scheduled-task-editor-dialog"
import {
  cloneScheduledTask,
  createEmptyScheduledTask,
  formatTaskDateTime,
  stateLabel,
} from "./scheduled-task-model"
import { ScheduledTaskTemplatesDialog } from "./scheduled-task-templates-dialog"

type TaskTreeNode = {
  name: string
  path: string
  folders: TaskTreeNode[]
  tasks: ScheduledTaskSummary[]
}

export function ScheduledTasksPage({ active }: { active: boolean }) {
  const [tasks, setTasks] = useState<ScheduledTaskSummary[]>([])
  const [folders, setFolders] = useState<string[]>([])
  const [loading, setLoading] = useState(true)
  const [query, setQuery] = useState("")
  const [editorOpen, setEditorOpen] = useState(false)
  const [editorTask, setEditorTask] = useState<ScheduledTaskEdit | null>(null)
  const [previousPath, setPreviousPath] = useState("")
  const [editorTitle, setEditorTitle] = useState<string>()
  const [editorMode, setEditorMode] = useState<"task" | "template">("task")
  const [editingTemplate, setEditingTemplate] = useState<ScheduledTaskTemplate | null>(null)
  const [templateEditorName, setTemplateEditorName] = useState("")
  const [editingPath, setEditingPath] = useState("")
  const [runningPath, setRunningPath] = useState("")
  const [deleteTask, setDeleteTask] = useState<ScheduledTaskSummary | null>(null)
  const [deleteTaskOpen, setDeleteTaskOpen] = useState(false)
  const [expandedFolders, setExpandedFolders] = useState<Set<string>>(() => new Set())
  const [templatesOpen, setTemplatesOpen] = useState(false)
  const [templateRefreshKey, setTemplateRefreshKey] = useState(0)
  const [templateSource, setTemplateSource] = useState<ScheduledTaskEdit | null>(null)
  const [templateName, setTemplateName] = useState("")
  const [templateSaveOpen, setTemplateSaveOpen] = useState(false)
  const [templateSaving, setTemplateSaving] = useState(false)
  const [folderParent, setFolderParent] = useState<string | null>(null)
  const [folderName, setFolderName] = useState("")
  const [folderOpen, setFolderOpen] = useState(false)
  const [folderSaving, setFolderSaving] = useState(false)
  const [deleteFolder, setDeleteFolder] = useState<TaskTreeNode | null>(null)
  const [deleteFolderOpen, setDeleteFolderOpen] = useState(false)
  const [historyEnabled, setHistoryEnabled] = useState<boolean | null>(null)
  const [historyStatusLoading, setHistoryStatusLoading] = useState(true)
  const [historyTask, setHistoryTask] = useState<ScheduledTaskSummary | null>(null)
  const [historyEvents, setHistoryEvents] = useState<ScheduledTaskHistoryEvent[]>([])
  const [historyLoading, setHistoryLoading] = useState(false)
  const [historyOpen, setHistoryOpen] = useState(false)
  const hasLoadedRef = useRef(false)

  const load = useCallback(async (silent = false) => {
    if (!silent) setLoading(true)
    try {
      const [nextTasks, nextFolders] = await Promise.all([
        ScheduledTaskService.List(),
        ScheduledTaskService.ListFolders(),
      ])
      setTasks(nextTasks ?? [])
      setFolders(nextFolders ?? [])
      hasLoadedRef.current = true
    } catch (reason) {
      showError(reason)
    } finally {
      if (!silent) setLoading(false)
    }
  }, [])

  const loadHistoryStatus = useCallback(async () => {
    setHistoryStatusLoading(true)
    try {
      setHistoryEnabled(await ScheduledTaskService.HistoryEnabled())
    } catch (reason) {
      showError(reason, "无法读取任务历史状态")
    } finally {
      setHistoryStatusLoading(false)
    }
  }, [])

  useEffect(() => {
    if (!active) return
    void load(hasLoadedRef.current)
    void loadHistoryStatus()
  }, [active, load, loadHistoryStatus])

  const taskTree = useMemo(() => {
    const keyword = query.trim().toLowerCase()
    const visibleTasks = keyword
      ? tasks.filter((task) => `${task.name}\n${task.path}\n${task.command}\n${task.arguments}`.toLowerCase().includes(keyword))
      : tasks
    const visibleFolders = keyword
      ? folders.filter((folder) => folder.toLowerCase().includes(keyword))
      : folders
    return buildTaskTree(visibleTasks, visibleFolders)
  }, [folders, query, tasks])

  const searchExpandedFolders = useMemo(() => {
    if (!query.trim()) return null
    const folders = new Set<string>()
    collectFolderPaths(taskTree, folders)
    return folders
  }, [query, taskTree])

  const openCreate = (folder?: string) => {
    setEditorMode("task")
    setEditingTemplate(null)
    setEditorTitle(undefined)
    const task = createEmptyScheduledTask()
    if (folder) task.folder = folder
    setEditorTask(task)
    setPreviousPath("")
    setEditorOpen(true)
  }

  const beginCreateFolder = (parent: string) => {
    setFolderParent(parent)
    setFolderName("")
    setFolderOpen(true)
  }

  const createFolder = async () => {
    if (folderParent === null || !folderName.trim()) return
    const path = joinTaskFolderPath(folderParent, folderName.trim())
    setFolderSaving(true)
    try {
      await ScheduledTaskService.CreateFolder(path)
      setFolderOpen(false)
      setExpandedFolders((current) => new Set(current).add(folderParent).add(path))
      await load(true)
    } catch (reason) {
      showError(reason, "无法创建文件夹")
    } finally {
      setFolderSaving(false)
    }
  }

  const confirmDeleteFolder = async () => {
    if (!deleteFolder) return
    setFolderSaving(true)
    try {
      await ScheduledTaskService.DeleteFolder(deleteFolder.path)
      setDeleteFolderOpen(false)
      await load(true)
    } catch (reason) {
      showError(reason, "无法删除文件夹")
    } finally {
      setFolderSaving(false)
    }
  }

  const openEdit = async (task: ScheduledTaskSummary) => {
    setEditingPath(task.path)
    try {
      const details = await ScheduledTaskService.Get(task.path)
      if (!details.editable) {
        showErrorMessage(details.readOnlyReason || "此任务暂不支持结构化编辑")
        return
      }
      setEditorMode("task")
      setEditingTemplate(null)
      setEditorTitle(undefined)
      setEditorTask(details.edit)
      setPreviousPath(task.path)
      setEditorOpen(true)
    } catch (reason) {
      showError(reason)
    } finally {
      setEditingPath("")
    }
  }

  const save = async (task: ScheduledTaskEdit, oldPath: string, password: string) => {
    if (editorMode === "template" && editingTemplate) {
      await ScheduledTaskTemplateService.Save({ ...editingTemplate, name: templateEditorName.trim(), task })
      setTemplateRefreshKey((current) => current + 1)
      return
    }
    await ScheduledTaskService.Save({ previousPath: oldPath, task, password })
    await load(tasks.length > 0)
  }

  const openFromTemplate = (template: ScheduledTaskTemplate) => {
    setEditorMode("task")
    setEditingTemplate(null)
    setEditorTitle("从模板新建")
    setEditorTask({ ...cloneScheduledTask(template.task), name: "" })
    setPreviousPath("")
    setEditorOpen(true)
  }

  const openTemplateEdit = (template: ScheduledTaskTemplate) => {
    setEditorMode("template")
    setEditingTemplate(template)
    setTemplateEditorName(template.name)
    setEditorTitle("编辑模板")
    setEditorTask(cloneScheduledTask(template.task))
    setPreviousPath("")
    setEditorOpen(true)
  }

  const openTemplateCreate = () => {
    const task = createEmptyScheduledTask()
    setEditorMode("template")
    setEditingTemplate({ id: "", name: "", task })
    setTemplateEditorName("")
    setEditorTitle("新建模板")
    setEditorTask(task)
    setPreviousPath("")
    setEditorOpen(true)
  }

  const beginSaveTemplate = async (task: ScheduledTaskSummary) => {
    setEditingPath(task.path)
    try {
      const details = await ScheduledTaskService.Get(task.path)
      if (!details.editable) {
        showErrorMessage(details.readOnlyReason || "此任务暂不支持保存为模板")
        return
      }
      setTemplateSource(details.edit)
      setTemplateName(task.name)
      setTemplateSaveOpen(true)
    } catch (reason) {
      showError(reason)
    } finally {
      setEditingPath("")
    }
  }

  const saveAsTemplate = async () => {
    if (!templateSource || !templateName.trim()) return
    setTemplateSaving(true)
    try {
      await ScheduledTaskTemplateService.Save({ id: "", name: templateName.trim(), task: templateSource })
      setTemplateSaveOpen(false)
    } catch (reason) {
      showError(reason, "无法保存")
    } finally {
      setTemplateSaving(false)
    }
  }

  const handleEditorOpenChange = (open: boolean) => {
    setEditorOpen(open)
  }

  const handleEditorOpenChangeComplete = (open: boolean) => {
    if (open) return
    setEditorTask(null)
    setPreviousPath("")
    setEditorTitle(undefined)
    setEditorMode("task")
    setEditingTemplate(null)
    setTemplateEditorName("")
  }

  const toggleEnabled = async (task: ScheduledTaskSummary, enabled: boolean) => {
    setTasks((current) => current.map((item) => (
      item.path === task.path ? { ...item, enabled, state: enabled ? "ready" : "disabled" } : item
    )))
    try {
      await ScheduledTaskService.SetEnabled(task.path, enabled)
    } catch (reason) {
      setTasks((current) => current.map((item) => (
        item.path === task.path ? task : item
      )))
      showError(reason)
    }
  }

  const run = async (task: ScheduledTaskSummary) => {
    setRunningPath(task.path)
    try {
      await ScheduledTaskService.Run(task.path)
      await load(tasks.length > 0)
    } catch (reason) {
      showError(reason)
    } finally {
      setRunningPath("")
    }
  }

  const stop = async (task: ScheduledTaskSummary) => {
    setRunningPath(task.path)
    try {
      await ScheduledTaskService.Stop(task.path)
      await load(tasks.length > 0)
    } catch (reason) {
      showError(reason)
    } finally {
      setRunningPath("")
    }
  }

  const exportXML = async (task: ScheduledTaskSummary) => {
    setRunningPath(task.path)
    try {
      await ScheduledTaskService.ExportXML(task.path)
    } catch (reason) {
      showError(reason)
    } finally {
      setRunningPath("")
    }
  }

  const confirmDelete = async () => {
    if (!deleteTask) return
    try {
      await ScheduledTaskService.Delete(deleteTask.path)
      setDeleteTaskOpen(false)
      await load(tasks.length > 0)
    } catch (reason) {
      showError(reason)
    }
  }

  const toggleHistory = async (enabled: boolean) => {
    const previous = historyEnabled
    setHistoryEnabled(enabled)
    setHistoryStatusLoading(true)
    try {
      await ScheduledTaskService.SetHistoryEnabled(enabled)
      setHistoryEnabled(await ScheduledTaskService.HistoryEnabled())
    } catch (reason) {
      setHistoryEnabled(previous)
      showError(reason, "无法修改任务历史状态")
    } finally {
      setHistoryStatusLoading(false)
    }
  }

  const openHistory = async (task: ScheduledTaskSummary) => {
    setHistoryTask(task)
    setHistoryEvents([])
    setHistoryOpen(true)
    setHistoryLoading(true)
    try {
      setHistoryEvents((await ScheduledTaskService.History(task.path, 100)) ?? [])
    } catch (reason) {
      showError(reason, "无法读取任务历史")
    } finally {
      setHistoryLoading(false)
    }
  }

  return (
    <div className="flex h-full min-h-0 w-full flex-col p-4 lg:p-6">
      <div className="flex shrink-0 flex-col gap-3">
        <div className="flex items-center justify-between gap-3">
          <div className="flex min-w-0 items-center gap-2">
            <SidebarTrigger className="-ml-1 shrink-0" />
            <h1 className="min-w-0 text-xl font-semibold tracking-tight">计划任务</h1>
          </div>
          <div className="flex shrink-0 items-center gap-2 whitespace-nowrap">
            <label className="flex h-9 cursor-pointer items-center gap-2 rounded-md border px-3 text-sm">
              <HistoryIcon className="size-4" />
              任务历史
              {historyEnabled === null ? (
                <span className="flex h-[14px] w-6 items-center justify-center text-muted-foreground" aria-label="正在读取任务历史状态">
                  {historyStatusLoading ? <LoaderCircleIcon className="size-3.5 animate-spin" /> : "—"}
                </span>
              ) : (
                <Switch
                  size="sm"
                  aria-label="任务历史"
                  checked={historyEnabled}
                  disabled={historyStatusLoading}
                  onCheckedChange={(checked) => void toggleHistory(checked)}
                />
              )}
            </label>
            <Button variant="outline" onClick={() => void load(tasks.length > 0)} disabled={loading}>
              <RefreshCwIcon data-icon="inline-start" />
              刷新
            </Button>
            <Button variant="outline" onClick={() => setTemplatesOpen(true)}>
              <CopyPlusIcon data-icon="inline-start" />
              模板
            </Button>
            <Button variant="outline" onClick={() => beginCreateFolder("\\")}>
              <FolderPlusIcon data-icon="inline-start" />
              新建文件夹
            </Button>
            <Button onClick={() => openCreate()}>
              <PlusIcon data-icon="inline-start" />
              添加任务
            </Button>
          </div>
        </div>

        <InputGroup className="w-full lg:max-w-sm">
          <InputGroupAddon>
            <SearchIcon />
          </InputGroupAddon>
          <InputGroupInput
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder="搜索名称或路径"
          />
        </InputGroup>
      </div>

      <ScrollArea className="min-h-0 flex-1">
        <div className="flex min-h-full flex-col pt-3 pr-3 pb-1">
        {loading ? (
          <div className="flex min-h-40 flex-1 items-center justify-center" aria-label="加载中">
            <LoaderCircleIcon className="size-5 animate-spin text-muted-foreground" />
          </div>
        ) : taskTree.tasks.length || taskTree.folders.length ? (
          <div className="overflow-hidden rounded-lg border bg-card">
            <div className="hidden h-8 grid-cols-[minmax(0,1fr)_8.5rem] items-center gap-3 border-b bg-muted/20 px-2.5 text-xs text-muted-foreground md:grid">
              <div>任务</div>
              <div>下次运行</div>
            </div>
            <TaskTree
              tree={taskTree}
              depth={0}
              expandedFolders={searchExpandedFolders ?? expandedFolders}
              onFolderToggle={(path) => setExpandedFolders((current) => toggleSetValue(current, path))}
              editingPath={editingPath}
              runningPath={runningPath}
              onEdit={(task) => void openEdit(task)}
              onRun={(task) => void run(task)}
              onStop={(task) => void stop(task)}
              onHistory={(task) => void openHistory(task)}
              onExportXML={(task) => void exportXML(task)}
              onEnabledChange={(task, enabled) => void toggleEnabled(task, enabled)}
              onSaveTemplate={(task) => void beginSaveTemplate(task)}
              onDelete={(task) => {
                setDeleteTask(task)
                setDeleteTaskOpen(true)
              }}
              onCreateTask={openCreate}
              onCreateFolder={beginCreateFolder}
              onDeleteFolder={(folder) => {
                setDeleteFolder(folder)
                setDeleteFolderOpen(true)
              }}
            />
          </div>
        ) : (
          <Empty className="min-h-48 border">
            <EmptyHeader>
              <EmptyTitle>{query ? "没有匹配的任务" : "没有计划任务"}</EmptyTitle>
            </EmptyHeader>
            {!query ? (
              <EmptyContent>
                <Button onClick={() => openCreate()}>
                  <PlusIcon data-icon="inline-start" />
                  添加任务
                </Button>
              </EmptyContent>
            ) : null}
          </Empty>
        )}
        </div>
      </ScrollArea>

      <ScheduledTaskEditorDialog
        open={editorOpen}
        task={editorTask}
        previousPath={previousPath}
        title={editorTitle}
        allowXMLImport={editorMode === "task" && !previousPath}
        templateName={editorMode === "template" ? templateEditorName : undefined}
        onTemplateNameChange={setTemplateEditorName}
        onOpenChange={handleEditorOpenChange}
        onOpenChangeComplete={handleEditorOpenChangeComplete}
        onSave={save}
      />

      <ScheduledTaskTemplatesDialog
        open={templatesOpen}
        refreshKey={templateRefreshKey}
        onOpenChange={setTemplatesOpen}
        onCreate={openTemplateCreate}
        onUse={openFromTemplate}
        onEdit={openTemplateEdit}
      />

      <Dialog
        open={folderOpen}
        onOpenChange={setFolderOpen}
        onOpenChangeComplete={(open) => {
          if (open) return
          setFolderParent(null)
          setFolderName("")
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>新建文件夹</DialogTitle>
          </DialogHeader>
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="task-folder-name">名称</FieldLabel>
              <Input
                id="task-folder-name"
                value={folderName}
                onChange={(event) => setFolderName(event.target.value)}
                autoFocus
              />
            </Field>
          </FieldGroup>
          <DialogFooter>
            <Button variant="outline" onClick={() => setFolderOpen(false)} disabled={folderSaving}>取消</Button>
            <Button onClick={() => void createFolder()} disabled={folderSaving || !folderName.trim()}>创建</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog
        open={deleteFolderOpen}
        onOpenChange={setDeleteFolderOpen}
        onOpenChangeComplete={(open) => !open && setDeleteFolder(null)}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>删除计划任务文件夹</DialogTitle>
            <DialogDescription>将递归删除“{deleteFolder?.path}”中的所有任务和子文件夹。</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleteFolderOpen(false)} disabled={folderSaving}>取消</Button>
            <Button onClick={() => void confirmDeleteFolder()} disabled={folderSaving}>删除</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog
        open={templateSaveOpen}
        onOpenChange={setTemplateSaveOpen}
        onOpenChangeComplete={(open) => {
          if (open) return
          setTemplateSource(null)
          setTemplateName("")
        }}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>保存为模板</DialogTitle>
          </DialogHeader>
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="task-template-name">模板名称</FieldLabel>
              <Input id="task-template-name" value={templateName} onChange={(event) => setTemplateName(event.target.value)} autoFocus />
            </Field>
          </FieldGroup>
          <DialogFooter>
            <Button variant="outline" onClick={() => setTemplateSaveOpen(false)} disabled={templateSaving}>取消</Button>
            <Button onClick={() => void saveAsTemplate()} disabled={templateSaving || !templateName.trim()}>保存</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog
        open={historyOpen}
        onOpenChange={setHistoryOpen}
        onOpenChangeComplete={(open) => {
          if (open) return
          setHistoryTask(null)
          setHistoryEvents([])
        }}
      >
        <DialogContent className="flex h-[min(680px,calc(100vh-2rem))] !w-[min(92vw,64rem)] min-w-0 !max-w-none flex-col overflow-hidden">
          <DialogHeader>
            <DialogTitle>任务历史{historyTask ? ` · ${historyTask.name}` : ""}</DialogTitle>
            <DialogDescription className="min-w-0 truncate">{historyTask?.path}</DialogDescription>
          </DialogHeader>

          <ScrollArea className="min-h-0 flex-1 rounded-lg border">
            {historyLoading ? (
              <div className="flex min-h-40 items-center justify-center" aria-label="加载历史记录">
                <LoaderCircleIcon className="size-5 animate-spin text-muted-foreground" />
              </div>
            ) : historyEvents.length ? (
              <Table>
                <TableHeader>
                  <TableRow>
                    <TableHead className="w-36">时间</TableHead>
                    <TableHead className="w-20">事件 ID</TableHead>
                    <TableHead>事件</TableHead>
                    <TableHead className="w-32">结果</TableHead>
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {historyEvents.map((event) => (
                    <TableRow key={event.recordId}>
                      <TableCell className="whitespace-nowrap text-xs tabular-nums text-muted-foreground">{formatHistoryDateTime(event.timestamp)}</TableCell>
                      <TableCell className="font-mono text-xs">{event.eventId}</TableCell>
                      <TableCell>
                        <div className="flex min-w-0 items-center gap-2">
                          <span className="font-medium">{event.message}</span>
                          {historyLevelLabel(event.level) ? <Badge variant="outline">{historyLevelLabel(event.level)}</Badge> : null}
                        </div>
                        {event.details ? <div className="mt-0.5 break-all text-xs text-muted-foreground">{event.details}</div> : null}
                      </TableCell>
                      <TableCell className="font-mono text-xs">{event.resultCode || "—"}</TableCell>
                    </TableRow>
                  ))}
                </TableBody>
              </Table>
            ) : (
              <div className="flex min-h-40 items-center justify-center text-sm text-muted-foreground">没有该任务的历史记录</div>
            )}
          </ScrollArea>
        </DialogContent>
      </Dialog>

      <Dialog
        open={deleteTaskOpen}
        onOpenChange={setDeleteTaskOpen}
        onOpenChangeComplete={(open) => !open && setDeleteTask(null)}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>删除计划任务</DialogTitle>
            <DialogDescription>删除“{deleteTask?.name}”？</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleteTaskOpen(false)}>取消</Button>
            <Button onClick={() => void confirmDelete()}>
              <Trash2Icon data-icon="inline-start" />
              删除
            </Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}

type TaskTreeProps = {
  tree: TaskTreeNode
  depth: number
  expandedFolders: Set<string>
  onFolderToggle: (path: string) => void
  editingPath: string
  runningPath: string
  onEdit: (task: ScheduledTaskSummary) => void
  onRun: (task: ScheduledTaskSummary) => void
  onStop: (task: ScheduledTaskSummary) => void
  onHistory: (task: ScheduledTaskSummary) => void
  onExportXML: (task: ScheduledTaskSummary) => void
  onEnabledChange: (task: ScheduledTaskSummary, enabled: boolean) => void
  onSaveTemplate: (task: ScheduledTaskSummary) => void
  onDelete: (task: ScheduledTaskSummary) => void
  onCreateTask: (folder: string) => void
  onCreateFolder: (parent: string) => void
  onDeleteFolder: (folder: TaskTreeNode) => void
}

function TaskTree({
  tree,
  depth,
  expandedFolders,
  onFolderToggle,
  editingPath,
  runningPath,
  onEdit,
  onRun,
  onStop,
  onHistory,
  onExportXML,
  onEnabledChange,
  onSaveTemplate,
  onDelete,
  onCreateTask,
  onCreateFolder,
  onDeleteFolder,
}: TaskTreeProps) {
  return (
    <div>
      {tree.folders.map((folder) => {
        const expanded = expandedFolders.has(folder.path)
        return (
          <div key={folder.path}>
            <ContextMenu>
              <ContextMenuTrigger
                className="flex h-8 w-full min-w-0 cursor-pointer items-center gap-1.5 border-b px-2.5 text-left text-sm hover:bg-muted/50"
                style={{ paddingLeft: `${depth * 16 + 10}px` }}
                onClick={() => onFolderToggle(folder.path)}
                aria-expanded={expanded}
              >
                <ChevronRightIcon className={`size-3.5 shrink-0 transition-transform ${expanded ? "rotate-90" : ""}`} />
                {expanded ? <FolderOpenIcon className="size-3.5 shrink-0" /> : <FolderIcon className="size-3.5 shrink-0" />}
                <TruncatedText value={folder.name} className="flex-1 font-medium" />
                <span className="shrink-0 text-xs text-muted-foreground">{countDirectChildren(folder)}</span>
              </ContextMenuTrigger>
              <ContextMenuContent className="w-40">
                <ContextMenuItem onClick={() => onCreateTask(folder.path)}>
                  <PlusIcon />
                  新建任务
                </ContextMenuItem>
                <ContextMenuItem onClick={() => onCreateFolder(folder.path)}>
                  <FolderPlusIcon />
                  新建文件夹
                </ContextMenuItem>
                <ContextMenuSeparator />
                <ContextMenuItem onClick={() => onDeleteFolder(folder)}>
                  <Trash2Icon />
                  删除
                </ContextMenuItem>
              </ContextMenuContent>
            </ContextMenu>
            {expanded ? (
              <TaskTree
                tree={folder}
                depth={depth + 1}
                expandedFolders={expandedFolders}
                onFolderToggle={onFolderToggle}
                editingPath={editingPath}
                runningPath={runningPath}
                onEdit={onEdit}
                onRun={onRun}
                onStop={onStop}
                onHistory={onHistory}
                onExportXML={onExportXML}
                onEnabledChange={onEnabledChange}
                onSaveTemplate={onSaveTemplate}
                onDelete={onDelete}
                onCreateTask={onCreateTask}
                onCreateFolder={onCreateFolder}
                onDeleteFolder={onDeleteFolder}
              />
            ) : null}
          </div>
        )
      })}

      {tree.tasks.map((task) => (
        <TaskRow
          key={task.path}
          task={task}
          depth={depth}
          editing={editingPath === task.path}
          running={runningPath === task.path}
          onEdit={() => onEdit(task)}
          onRun={() => onRun(task)}
          onStop={() => onStop(task)}
          onHistory={() => onHistory(task)}
          onExportXML={() => onExportXML(task)}
          onEnabledChange={(enabled) => onEnabledChange(task, enabled)}
          onSaveTemplate={() => onSaveTemplate(task)}
          onDelete={() => onDelete(task)}
        />
      ))}
    </div>
  )
}

type TaskRowProps = {
  task: ScheduledTaskSummary
  depth: number
  editing: boolean
  running: boolean
  onEdit: () => void
  onRun: () => void
  onStop: () => void
  onHistory: () => void
  onExportXML: () => void
  onEnabledChange: (enabled: boolean) => void
  onSaveTemplate: () => void
  onDelete: () => void
}

function TaskRow({
  task,
  depth,
  editing,
  running,
  onEdit,
  onRun,
  onStop,
  onHistory,
  onExportXML,
  onEnabledChange,
  onSaveTemplate,
  onDelete,
}: TaskRowProps) {
  const status = stateLabel(task.state)
  const actionSummary = [task.command, task.arguments].filter(Boolean).join(" ")
  return (
    <ContextMenu>
      <ContextMenuTrigger
        className="grid h-8 min-w-0 cursor-pointer grid-cols-[minmax(0,1fr)] items-center gap-2 border-b pr-2.5 hover:bg-muted/30 md:grid-cols-[minmax(0,1fr)_8.5rem] md:gap-3"
        style={{ paddingLeft: `${depth * 16 + 32}px` }}
        onClick={onEdit}
      >
        <span className="flex min-w-0 items-center gap-2">
          <TruncatedText value={task.name} className="max-w-[18rem] shrink-0 text-sm" />
          {status ? (
            <Badge variant={task.state === "running" ? "secondary" : "outline"}>{status}</Badge>
          ) : null}
          {actionSummary ? (
            <TruncatedText value={actionSummary} className="min-w-0 flex-1 font-mono text-xs text-muted-foreground" />
          ) : null}
        </span>
        <span className="hidden text-xs tabular-nums text-muted-foreground md:block">
          {formatTaskDateTime(task.nextRunTime)}
        </span>
      </ContextMenuTrigger>

      <ContextMenuContent className="w-40">
        <ContextMenuItem onClick={onRun} disabled={running || !task.enabled}>
          <PlayIcon />
          运行
        </ContextMenuItem>
        <ContextMenuItem onClick={onStop} disabled={running || task.state !== "running"}>
          <SquareIcon />
          停止
        </ContextMenuItem>
        <ContextMenuItem onClick={() => onEnabledChange(!task.enabled)} disabled={running}>
          <PowerIcon />
          {task.enabled ? "禁用" : "启用"}
        </ContextMenuItem>
        <ContextMenuSeparator />
        <ContextMenuItem onClick={onEdit} disabled={editing || running}>
          <PencilIcon />
          编辑
        </ContextMenuItem>
        <ContextMenuItem onClick={onHistory}>
          <HistoryIcon />
          历史记录
        </ContextMenuItem>
        <ContextMenuItem onClick={onExportXML} disabled={running}>
          <FileDownIcon />
          导出 XML
        </ContextMenuItem>
        <ContextMenuItem onClick={onSaveTemplate} disabled={running}>
          <CopyPlusIcon />
          保存为模板
        </ContextMenuItem>
        <ContextMenuSeparator />
        <ContextMenuItem onClick={onDelete} disabled={running}>
          <Trash2Icon />
          删除
        </ContextMenuItem>
      </ContextMenuContent>
    </ContextMenu>
  )
}

function buildTaskTree(tasks: ScheduledTaskSummary[], folderPaths: string[]): TaskTreeNode {
  const root: TaskTreeNode = { name: "", path: "\\", folders: [], tasks: [] }

  for (const folderPath of folderPaths) {
    ensureTaskTreeFolder(root, folderPath)
  }

  for (const task of tasks) {
    ensureTaskTreeFolder(root, task.folder).tasks.push(task)
  }

  sortTaskTree(root)
  return root
}

function ensureTaskTreeFolder(root: TaskTreeNode, folderPath: string): TaskTreeNode {
  const segments = folderPath.split("\\").filter(Boolean)
  let node = root
  let path = ""
  for (const segment of segments) {
    path += `\\${segment}`
    let child = node.folders.find((folder) => folder.name.toLowerCase() === segment.toLowerCase())
    if (!child) {
      child = { name: segment, path, folders: [], tasks: [] }
      node.folders.push(child)
    }
    node = child
  }
  return node
}

function sortTaskTree(node: TaskTreeNode) {
  node.folders.sort((a, b) => a.name.localeCompare(b.name))
  node.tasks.sort((a, b) => a.name.localeCompare(b.name))
  node.folders.forEach(sortTaskTree)
}

function countDirectChildren(node: TaskTreeNode): number {
  return node.tasks.length + node.folders.length
}

function joinTaskFolderPath(parent: string, name: string): string {
  const cleanName = name.trim().replaceAll("/", "\\").replace(/^\\+|\\+$/g, "")
  return parent === "\\" ? `\\${cleanName}` : `${parent}\\${cleanName}`
}

function collectFolderPaths(node: TaskTreeNode, result: Set<string>) {
  for (const folder of node.folders) {
    result.add(folder.path)
    collectFolderPaths(folder, result)
  }
}

function toggleSetValue(current: Set<string>, value: string): Set<string> {
  const next = new Set(current)
  if (next.has(value)) next.delete(value)
  else next.add(value)
  return next
}

function formatHistoryDateTime(value: string): string {
  if (!value) return "—"
  const date = new Date(value)
  if (Number.isNaN(date.getTime())) return value
  const pad = (part: number) => String(part).padStart(2, "0")
  return `${pad(date.getMonth() + 1)}/${pad(date.getDate())} ${pad(date.getHours())}:${pad(date.getMinutes())}:${pad(date.getSeconds())}`
}

function historyLevelLabel(level: number): string {
  switch (level) {
    case 1:
      return "关键"
    case 2:
      return "错误"
    case 3:
      return "警告"
    default:
      return ""
  }
}

