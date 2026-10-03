import { useCallback, useEffect, useMemo, useRef, useState } from "react"
import {
  CheckIcon,
  CopyIcon,
  FileCode2Icon,
  FolderIcon,
  FolderOpenIcon,
  ListTreeIcon,
  LoaderCircleIcon,
  MonitorIcon,
  PencilIcon,
  PlusIcon,
  PowerIcon,
  RefreshCwIcon,
  Rows3Icon,
  SearchIcon,
  TerminalSquareIcon,
  Trash2Icon,
  XIcon,
} from "lucide-react"

import {
  ContextMenuService,
  type ContextMenuStatus,
  type MenuEntry,
} from "../../../bindings/win-admin"
import { Button } from "@/components/ui/button"
import { Badge } from "@/components/ui/badge"
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuSub,
  ContextMenuSubContent,
  ContextMenuSubTrigger,
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
  EmptyDescription,
  EmptyHeader,
  EmptyMedia,
  EmptyTitle,
} from "@/components/ui/empty"
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from "@/components/ui/input-group"
import { ScrollArea } from "@/components/ui/scroll-area"
import { SidebarTrigger } from "@/components/ui/sidebar"
import { Switch } from "@/components/ui/switch"
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { TruncatedText } from "@/components/ui/truncated-text"
import { showError, showErrorMessage } from "@/lib/error-toast"
import { MenuEditorDialog } from "./menu-editor-dialog"
import { MenuIconPreview } from "./menu-icon-preview"
import {
  cloneEntry,
  countChildren,
  createEmptyEntry,
  normalizeEntry,
  shellMenuDisplayLabel,
  TARGET_LABEL,
  type TargetType,
} from "./menu-model"

export function ContextMenuPage({ active }: { active: boolean }) {
  const [target, setTarget] = useState<TargetType>("file")
  const [entries, setEntries] = useState<MenuEntry[]>([])
  const [status, setStatus] = useState<ContextMenuStatus | null>(null)
  const [loading, setLoading] = useState(true)
  const [query, setQuery] = useState("")
  const [editorOpen, setEditorOpen] = useState(false)
  const [editorEntry, setEditorEntry] = useState<MenuEntry | null>(null)
  const [previousTarget, setPreviousTarget] = useState("")
  const [previousId, setPreviousId] = useState("")
  const [deleteEntry, setDeleteEntry] = useState<MenuEntry | null>(null)
  const [deleteOpen, setDeleteOpen] = useState(false)
  const cacheRef = useRef(new Map<TargetType, MenuEntry[]>())
  const adminWarningShownRef = useRef(false)

  const load = useCallback(async (nextTarget: TargetType, silent = false) => {
    if (!silent) setLoading(true)
    try {
      const [menuEntries, currentStatus] = await Promise.all([
        ContextMenuService.List(nextTarget),
        ContextMenuService.Status(),
      ])
      const normalized = (menuEntries ?? []).map(normalizeEntry)
      cacheRef.current.set(nextTarget, normalized)
      setEntries(normalized)
      setStatus(currentStatus)
      if (!currentStatus.elevated && !adminWarningShownRef.current) {
        adminWarningShownRef.current = true
        showErrorMessage(
          "当前进程未提升。正式 Windows 构建已配置 requireAdministrator；开发模式请从管理员终端启动。",
          "需要管理员权限",
        )
      }
    } catch (reason) {
      showError(reason)
    } finally {
      if (!silent) setLoading(false)
    }
  }, [])

  useEffect(() => {
    if (!active) return
    const cached = cacheRef.current.get(target)
    if (cached) {
      setEntries(cached)
      setLoading(false)
      void load(target, true)
    } else {
      void load(target)
    }
  }, [active, load, target])

  const visibleEntries = useMemo(() => {
    const keyword = query.trim().toLowerCase()
    if (!keyword) return entries
    return entries.filter((entry) =>
      [entry.label, entry.displayLabel, shellMenuDisplayLabel(entry.displayLabel || entry.label), entry.command, entry.icon, entry.id]
        .join("\n")
        .toLowerCase()
        .includes(keyword),
    )
  }, [entries, query])
  const entryGroups = [
    { label: "可编辑", entries: visibleEntries.filter((entry) => entry.canEdit && !entry.readOnly) },
    { label: "不可编辑", entries: visibleEntries.filter((entry) => !entry.canEdit || entry.readOnly) },
  ]

  const openCreate = () => {
    setEditorEntry(createEmptyEntry(target))
    setPreviousId("")
    setPreviousTarget("")
    setEditorOpen(true)
  }

  const openEdit = (entry: MenuEntry) => {
    setEditorEntry(cloneEntry(entry))
    setPreviousId(entry.id)
    setPreviousTarget(entry.target)
    setEditorOpen(true)
  }

  const save = async (entry: MenuEntry, oldTarget: string, oldId: string) => {
    await ContextMenuService.Save({
      previousTarget: oldTarget,
      previousId: oldId,
      entry,
    })
    if (entry.target !== target) {
      setTarget(entry.target as TargetType)
    } else {
      await load(target, true)
    }
  }

  const handleEditorOpenChange = (open: boolean) => {
    setEditorOpen(open)
  }

  const handleEditorOpenChangeComplete = (open: boolean) => {
    if (open) return
    setEditorEntry(null)
    setPreviousId("")
    setPreviousTarget("")
  }

  const toggleEnabled = async (entry: MenuEntry, enabled: boolean) => {
    if (!entry.canEnable) return
    setEntries((current) => current.map((item) => (item.id === entry.id ? { ...item, enabled } : item)))
    try {
      await ContextMenuService.SetEnabled(entry.registryPath, enabled)
      await load(target, true)
    } catch (reason) {
      setEntries((current) =>
        current.map((item) => (item.id === entry.id ? { ...item, enabled: entry.enabled } : item)),
      )
      showError(reason)
    }
  }

  const changePosition = async (entry: MenuEntry, position: "top" | "normal" | "bottom") => {
    if (!entry.canPosition || entry.position === position) return
    const previousPosition = entry.position
    setEntries((current) => current.map((item) => (
      item.id === entry.id ? { ...item, position } : item
    )))
    try {
      await ContextMenuService.SetPosition(entry.registryPath, position)
      await load(target, true)
    } catch (reason) {
      setEntries((current) => current.map((item) => (
        item.id === entry.id ? { ...item, position: previousPosition } : item
      )))
      showError(reason)
    }
  }

  const changeShiftOnly = async (entry: MenuEntry, shiftOnly: boolean) => {
    if (!entry.canShift || entry.shiftOnly === shiftOnly) return
    setEntries((current) => current.map((item) => (
      item.id === entry.id ? { ...item, shiftOnly } : item
    )))
    try {
      await ContextMenuService.SetShiftOnly(entry.registryPath, shiftOnly)
      await load(target, true)
    } catch (reason) {
      setEntries((current) => current.map((item) => (
        item.id === entry.id ? { ...item, shiftOnly: entry.shiftOnly } : item
      )))
      showError(reason)
    }
  }

  const confirmDelete = async () => {
    if (!deleteEntry) return
    try {
      await ContextMenuService.Delete(deleteEntry.registryPath)
      setDeleteOpen(false)
      await load(target, true)
    } catch (reason) {
      showError(reason)
    }
  }

  const openRegistry = async (entry: MenuEntry) => {
    try {
      await ContextMenuService.OpenRegistry(entry.registryPath)
    } catch (reason) {
      showError(reason, "无法打开注册表位置")
    }
  }

  return (
    <div className="flex h-full min-h-0 w-full flex-col p-4 lg:p-6">
      <div className="flex shrink-0 flex-col gap-3">
      <div className="flex items-center justify-between gap-3">
        <div className="flex min-w-0 items-center gap-2">
          <SidebarTrigger className="-ml-1 shrink-0" />
          <h1 className="min-w-0 text-xl font-semibold tracking-tight">上下文菜单</h1>
        </div>
        <div className="flex shrink-0 items-center gap-2 whitespace-nowrap">
          <Button variant="outline" onClick={() => void load(target, entries.length > 0)} disabled={loading}>
            <RefreshCwIcon data-icon="inline-start" />
            刷新
          </Button>
          <Button onClick={openCreate} disabled={status?.elevated === false}>
            <PlusIcon data-icon="inline-start" />
            添加菜单
          </Button>
        </div>
      </div>

      <div className="flex flex-wrap items-center gap-2 rounded-lg border bg-card p-2">
        <Tabs className="min-w-[30rem] flex-[1_1_32rem]" value={target} onValueChange={(value) => setTarget(value as TargetType)}>
          <TabsList className="grid w-full grid-cols-4">
            <TabsTrigger value="file">
              <FileCode2Icon data-icon="inline-start" />
              文件
            </TabsTrigger>
            <TabsTrigger value="directory">
              <FolderIcon data-icon="inline-start" />
              目录
            </TabsTrigger>
            <TabsTrigger value="background">
              <FolderOpenIcon data-icon="inline-start" />
              目录背景
            </TabsTrigger>
            <TabsTrigger value="desktop-background">
              <MonitorIcon data-icon="inline-start" />
              桌面背景
            </TabsTrigger>
          </TabsList>
        </Tabs>

        <InputGroup className="w-full flex-[1_1_18rem]">
          <InputGroupAddon>
            <SearchIcon />
          </InputGroupAddon>
          <InputGroupInput
            value={query}
            onChange={(event) => setQuery(event.target.value)}
            placeholder="搜索名称、命令或 ID"
          />
        </InputGroup>
      </div>

      </div>

      <ScrollArea className="-mr-4 min-h-0 flex-1 pr-4 lg:-mr-6 lg:pr-6">
        <div className="flex min-h-full flex-col pt-3 pb-1">
      {loading ? (
        <div className="flex min-h-40 flex-1 items-center justify-center" aria-label="加载中">
          <LoaderCircleIcon className="size-5 animate-spin text-muted-foreground" />
        </div>
      ) : visibleEntries.length ? (
        <div className="flex flex-col gap-4">
          {entryGroups.filter((group) => group.entries.length > 0).map((group) => (
            <section key={group.label} className="flex flex-col gap-2">
              <h2 className="px-1 text-xs font-medium text-muted-foreground">{group.label}</h2>
              <div className="overflow-hidden rounded-lg border bg-card">
                {group.entries.map((entry) => (
                  <MenuCard
                    key={entry.registryPath}
                    entry={entry}
                    onEdit={() => openEdit(entry)}
                    onDelete={() => {
                      setDeleteEntry(entry)
                      setDeleteOpen(true)
                    }}
                    onOpenRegistry={() => void openRegistry(entry)}
                    onEnabledChange={(enabled) => void toggleEnabled(entry, enabled)}
                    onPositionChange={(position) => void changePosition(entry, position)}
                    onShiftOnlyChange={(shiftOnly) => void changeShiftOnly(entry, shiftOnly)}
                  />
                ))}
              </div>
            </section>
          ))}
        </div>
      ) : (
        <Empty className="min-h-72 border">
          <EmptyHeader>
            <EmptyMedia variant="icon">
              <TerminalSquareIcon />
            </EmptyMedia>
            <EmptyTitle>{query ? "没有匹配的菜单" : "这里还没有上下文菜单"}</EmptyTitle>
            <EmptyDescription>
              {query ? "换一个关键字，或清空搜索条件。" : `为${TARGET_LABEL[target]}添加一个命令或多级菜单。`}
            </EmptyDescription>
          </EmptyHeader>
          {!query ? (
            <EmptyContent>
              <Button onClick={openCreate} disabled={status?.elevated === false}>
                <PlusIcon data-icon="inline-start" />
                添加菜单
              </Button>
            </EmptyContent>
          ) : null}
        </Empty>
      )}
        </div>
      </ScrollArea>

      <MenuEditorDialog
        open={editorOpen}
        entry={editorEntry}
        previousTarget={previousTarget}
        previousId={previousId}
        onOpenChange={handleEditorOpenChange}
        onOpenChangeComplete={handleEditorOpenChangeComplete}
        onSave={save}
      />

      <Dialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        onOpenChangeComplete={(open) => !open && setDeleteEntry(null)}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>删除“{deleteEntry?.label}”？</DialogTitle>
            <DialogDescription>
              将删除对应的 Explorer 上下文菜单注册项。
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleteOpen(false)}>
              <XIcon data-icon="inline-start" />
              取消
            </Button>
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

type MenuCardProps = {
  entry: MenuEntry
  onEdit: () => void
  onDelete: () => void
  onOpenRegistry: () => void
  onEnabledChange: (enabled: boolean) => void
  onPositionChange: (position: "top" | "normal" | "bottom") => void
  onShiftOnlyChange: (shiftOnly: boolean) => void
}

function MenuCard({ entry, onEdit, onDelete, onOpenRegistry, onEnabledChange, onPositionChange, onShiftOnlyChange }: MenuCardProps) {
  const command = entry.command
  const childCount = countChildren(entry)
  const displayLabel = shellMenuDisplayLabel(entry.displayLabel || entry.label)
  const canEdit = entry.canEdit && !entry.readOnly
  const pendingEditRef = useRef(false)

  return (
    <ContextMenu onOpenChangeComplete={(open) => {
      if (!open && pendingEditRef.current) {
        pendingEditRef.current = false
        onEdit()
      }
    }}>
      <ContextMenuTrigger className="flex h-9 min-w-0 items-stretch border-b last:border-b-0 hover:bg-muted/30">
        <button
          type="button"
          className="flex min-w-0 flex-1 cursor-pointer items-center gap-2 px-2.5 text-left disabled:cursor-default"
          onClick={onEdit}
          disabled={!canEdit}
        >
          <div className="flex size-6 shrink-0 items-center justify-center">
            <MenuIconPreview
              icon={entry.icon}
              command={entry.command}
              kind={entry.kind}
              className="size-4"
            />
          </div>
          <TruncatedText value={displayLabel} className="max-w-[16rem] shrink-0 text-sm font-medium" />
          {entry.shiftOnly ? <Badge variant="outline" className="h-4 px-1.5 text-[10px]">Shift</Badge> : null}
          {entry.kind === "submenu" ? (
            <span className="min-w-0 flex-1 truncate font-mono text-xs text-muted-foreground">
              {childCount} 个子项
            </span>
          ) : command ? (
            <TruncatedText value={command} className="min-w-0 flex-1 font-mono text-xs text-muted-foreground" />
          ) : (
            <span className="min-w-0 flex-1 truncate font-mono text-xs text-muted-foreground">
              未读取到命令
            </span>
          )}
          <span className="flex w-8 shrink-0 justify-end text-[11px] text-muted-foreground">
            {entry.position === "top" ? "置顶" : entry.position === "bottom" ? "置底" : ""}
          </span>
        </button>
        <div className="flex shrink-0 items-center pr-2.5">
          <Switch
            size="sm"
            checked={entry.enabled}
            onCheckedChange={onEnabledChange}
            disabled={!entry.canEnable}
            aria-label={`${entry.enabled ? "禁用" : "启用"}${displayLabel}`}
          />
        </div>
      </ContextMenuTrigger>
      <ContextMenuContent className="min-w-max">
        {canEdit ? (
          <>
            <ContextMenuItem onClick={() => { pendingEditRef.current = true }}>
              <PencilIcon />
              编辑
            </ContextMenuItem>
            <ContextMenuSeparator />
          </>
        ) : null}
        <ContextMenuItem onClick={() => onEnabledChange(!entry.enabled)} disabled={!entry.canEnable}>
          <PowerIcon />
          {entry.enabled ? "禁用" : "启用"}
        </ContextMenuItem>
        <ContextMenuItem onClick={() => onShiftOnlyChange(!entry.shiftOnly)} disabled={!entry.canShift}>
          <CheckIcon className={entry.shiftOnly ? "" : "invisible"} />
          仅按住 Shift 显示
        </ContextMenuItem>
        <ContextMenuSub>
          <ContextMenuSubTrigger disabled={!entry.canPosition}>
            <Rows3Icon />
            菜单位置
          </ContextMenuSubTrigger>
          <ContextMenuSubContent>
            <ContextMenuItem onClick={() => onPositionChange("top")}>
              <CheckIcon className={entry.position === "top" ? "" : "invisible"} />
              置顶
            </ContextMenuItem>
            <ContextMenuItem onClick={() => onPositionChange("normal")}>
              <CheckIcon className={entry.position === "normal" ? "" : "invisible"} />
              默认
            </ContextMenuItem>
            <ContextMenuItem onClick={() => onPositionChange("bottom")}>
              <CheckIcon className={entry.position === "bottom" ? "" : "invisible"} />
              置底
            </ContextMenuItem>
          </ContextMenuSubContent>
        </ContextMenuSub>
        <ContextMenuSeparator />
        <ContextMenuItem onClick={onOpenRegistry}>
          <ListTreeIcon />
          打开注册表位置
        </ContextMenuItem>
        <ContextMenuItem
          className="whitespace-nowrap"
          onClick={() => navigator.clipboard.writeText(entry.registryPath)}
        >
          <CopyIcon />
          复制注册表路径
        </ContextMenuItem>
        <ContextMenuSeparator />
        <ContextMenuItem onClick={onDelete} disabled={!entry.canDelete}>
          <Trash2Icon />
          删除
        </ContextMenuItem>
      </ContextMenuContent>
    </ContextMenu>
  )
}


