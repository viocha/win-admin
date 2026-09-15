import { useEffect, useState } from "react"
import {
  AlignJustifyIcon,
  ArrowDownIcon,
  ArrowUpIcon,
  FileCode2Icon,
  FileImageIcon,
  FolderIcon,
  FolderOpenIcon,
  FolderTreeIcon,
  PencilIcon,
  PlusIcon,
  SaveIcon,
  SparklesIcon,
  Trash2Icon,
  XIcon,
} from "lucide-react"
import { cn } from "cn"

import { ContextMenuService, type MenuEntry } from "../../../bindings/win-admin"
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
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import {
  Field,
  FieldGroup,
  FieldLabel,
} from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
} from "@/components/ui/input-group"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Switch } from "@/components/ui/switch"
import { Textarea } from "@/components/ui/textarea"
import { Tooltip, TooltipContent, TooltipTrigger } from "@/components/ui/tooltip"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { showError, showErrorMessage } from "@/lib/error-toast"
import { MenuIconPreview } from "./menu-icon-preview"
import {
  cloneEntry,
  createEmptyEntry,
  type MenuKind,
  TARGETS,
  type TargetType,
  validateEntry,
} from "./menu-model"

type MenuEditorDialogProps = {
  open: boolean
  entry: MenuEntry | null
  previousTarget: string
  previousId: string
  onOpenChange: (open: boolean) => void
  onOpenChangeComplete?: (open: boolean) => void
  onSave: (entry: MenuEntry, previousTarget: string, previousId: string) => Promise<void>
}

export function MenuEditorDialog({
  open,
  entry,
  previousTarget,
  previousId,
  onOpenChange,
  onOpenChangeComplete,
  onSave,
}: MenuEditorDialogProps) {
  const [draft, setDraft] = useState<MenuEntry | null>(entry ? cloneEntry(entry) : null)
  const [saving, setSaving] = useState(false)

  useEffect(() => {
    if (!open) return
    setDraft(entry ? cloneEntry(entry) : null)
  }, [entry, open])

  const handleOpenChangeComplete = (nextOpen: boolean) => {
    if (!nextOpen) setDraft(null)
    onOpenChangeComplete?.(nextOpen)
  }

  if (!draft) return null

  const submit = async () => {
    const validationError = validateEntry(draft)
    if (validationError) {
      showErrorMessage(validationError, "无法保存")
      return
    }
    setSaving(true)
    try {
      await onSave(draft, previousTarget, previousId)
      onOpenChange(false)
    } catch (reason) {
      showError(reason, "无法保存")
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange} onOpenChangeComplete={handleOpenChangeComplete}>
      <DialogContent className="flex h-[min(760px,calc(100vh-2rem))] !w-[min(92vw,64rem)] min-w-0 !max-w-none flex-col overflow-hidden">
        <DialogHeader>
          <DialogTitle>{previousId ? "编辑上下文菜单" : "添加上下文菜单"}</DialogTitle>
        </DialogHeader>

        <ScrollArea className="min-h-0 min-w-0 flex-1 pr-3">
          <div className="flex min-w-0 flex-col gap-3 p-1">
            <FieldGroup className="min-w-0 gap-3">
              <Field className="min-w-0">
                <FieldLabel htmlFor="menu-label">菜单名称</FieldLabel>
                <Input
                  id="menu-label"
                  value={draft.label}
                  onChange={(event) => setDraft({ ...draft, label: event.target.value })}
                  placeholder="例如：使用 VS Code 打开"
                />
              </Field>

              <FieldGroup className="flex min-w-0 flex-row flex-nowrap items-start gap-4">
                <Field className="!w-fit shrink-0">
                  <FieldLabel>目标</FieldLabel>
                  <ToggleGroup
                    value={[draft.target]}
                    onValueChange={(value) => {
                      const next = value[0] as TargetType | undefined
                      if (next) setDraft({ ...draft, target: next })
                    }}
                    variant="outline"
                    spacing={0}
                  >
                    {TARGETS.map((target) => (
                      <ToggleGroupItem key={target.value} value={target.value}>
                        {target.value === "file" ? (
                          <FileCode2Icon data-icon="inline-start" />
                        ) : target.value === "directory" ? (
                          <FolderIcon data-icon="inline-start" />
                        ) : (
                          <FolderOpenIcon data-icon="inline-start" />
                        )}
                        {target.label}
                      </ToggleGroupItem>
                    ))}
                  </ToggleGroup>
                </Field>

                <Field className="!w-fit shrink-0">
                  <FieldLabel>类型</FieldLabel>
                  <ToggleGroup
                    value={[draft.kind]}
                    onValueChange={(value) => {
                      const next = value[0] as MenuKind | undefined
                      if (next) setDraft({ ...draft, kind: next })
                    }}
                    variant="outline"
                    spacing={0}
                  >
                    <ToggleGroupItem value="command">
                      <FileCode2Icon data-icon="inline-start" />
                      命令
                    </ToggleGroupItem>
                    <ToggleGroupItem value="submenu">
                      <FolderTreeIcon data-icon="inline-start" />
                      多级菜单
                    </ToggleGroupItem>
                  </ToggleGroup>
                </Field>

                <Field className="!w-fit shrink-0">
                  <FieldLabel>位置</FieldLabel>
                  <ToggleGroup
                    value={[draft.position || "normal"]}
                    onValueChange={(value) => value[0] && setDraft({ ...draft, position: value[0] })}
                    variant="outline"
                    spacing={0}
                  >
                    <ToggleGroupItem value="top">
                      <ArrowUpIcon data-icon="inline-start" />
                      置顶
                    </ToggleGroupItem>
                    <ToggleGroupItem value="normal">
                      <AlignJustifyIcon data-icon="inline-start" />
                      默认
                    </ToggleGroupItem>
                    <ToggleGroupItem value="bottom">
                      <ArrowDownIcon data-icon="inline-start" />
                      置底
                    </ToggleGroupItem>
                  </ToggleGroup>
                </Field>
              </FieldGroup>

			  <label className="flex min-h-9 cursor-pointer items-center justify-between gap-3 rounded-lg border px-3 text-sm transition-colors hover:bg-muted/30">
				<span>仅按住 Shift 时显示</span>
				<Switch
				  size="sm"
				  checked={draft.shiftOnly}
				  onCheckedChange={(shiftOnly) => setDraft({ ...draft, shiftOnly })}
				/>
			  </label>

              <IconField
                icon={draft.icon}
                command={draft.command}
                kind={draft.kind as MenuKind}
                onChange={(icon) => setDraft({ ...draft, icon })}
              />
            </FieldGroup>

            {draft.kind === "command" ? (
              <CommandField
                value={draft.command}
                target={draft.target}
                onChange={(command) => setDraft({ ...draft, command, commandTemplate: command })}
              />
            ) : (
              <SubmenuTreeEditor
                nodes={draft.children ?? []}
                target={draft.target as TargetType}
                onChange={(children) => setDraft({ ...draft, children })}
              />
            )}
          </div>
        </ScrollArea>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={saving}>
            <XIcon data-icon="inline-start" />
            取消
          </Button>
          <Button onClick={submit} disabled={saving}>
            <SaveIcon data-icon="inline-start" />
            保存
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

type IconFieldProps = {
  icon: string
  command: string
  kind: MenuKind
  onChange: (icon: string) => void
}

function IconField({ icon, command, kind, onChange }: IconFieldProps) {
  const infer = async () => {
    if (!command.trim()) return
    const result = await ContextMenuService.InferIcon(command)
    if (result) onChange(result)
  }

  const pick = async () => {
    const result = await ContextMenuService.PickIconFile()
    if (result) onChange(result)
  }

  return (
    <Field className="min-w-0">
      <FieldLabel>图标</FieldLabel>
      <div className="flex min-w-0 items-center gap-2">
        <div className="flex size-8 shrink-0 items-center justify-center rounded-md border bg-muted/40">
          <MenuIconPreview icon={icon} command={command} kind={kind} className="size-5" />
        </div>
        <InputGroup className="min-w-0 flex-1">
          <InputGroupInput
            value={icon}
            onChange={(event) => onChange(event.target.value)}
            placeholder="EXE / DLL / ICO，可留空"
          />
          <InputGroupAddon align="inline-end">
            {kind === "command" ? (
              <Tooltip>
                <TooltipTrigger
                  render={
                    <InputGroupButton
                      onClick={() => void infer()}
                      disabled={!command.trim()}
                      aria-label="推断图标"
                    >
                      <SparklesIcon />
                    </InputGroupButton>
                  }
                />
                <TooltipContent>根据命令自动推断图标</TooltipContent>
              </Tooltip>
            ) : null}
			<Tooltip>
              <TooltipTrigger
                render={
                  <InputGroupButton onClick={() => void pick()} aria-label="选择图标文件">
                    <FileImageIcon />
                  </InputGroupButton>
                }
              />
              <TooltipContent>选择图标文件</TooltipContent>
            </Tooltip>
          </InputGroupAddon>
        </InputGroup>
      </div>
    </Field>
  )
}

type CommandFieldProps = {
  value: string
  target: string
  onChange: (value: string) => void
}

function CommandField({ value, target, onChange }: CommandFieldProps) {
  const pathPlaceholder = target === "background" ? "%V" : "%1"

  const insertPath = () => {
    const suffix = value.trim() ? `${value} "${pathPlaceholder}"` : `"${pathPlaceholder}"`
    onChange(suffix)
  }

  return (
    <Field className="min-w-0">
      <div className="flex items-center justify-between gap-3">
        <FieldLabel>命令</FieldLabel>
        <Button type="button" variant="outline" size="xs" onClick={insertPath}>
          插入路径
        </Button>
      </div>
      <Textarea
        rows={4}
        value={value}
        onChange={(event) => onChange(event.target.value)}
        placeholder={`例如："C:\\Program Files\\App\\app.exe" "${pathPlaceholder}"`}
        className="font-mono text-xs"
      />
    </Field>
  )
}

type NodePath = number[]

type SubmenuTreeEditorProps = {
  nodes: MenuEntry[]
  target: TargetType
  onChange: (nodes: MenuEntry[]) => void
}

function SubmenuTreeEditor({ nodes, target, onChange }: SubmenuTreeEditorProps) {
  const [selectedKey, setSelectedKey] = useState("")
  const [expandedKeys, setExpandedKeys] = useState<Set<string>>(() => new Set())

  const selectedPath = selectedKey ? pathFromKey(selectedKey) : null
  const selectedNode = selectedPath ? getNodeAt(nodes, selectedPath) : null

  useEffect(() => {
    if (selectedKey && !selectedNode) setSelectedKey("")
  }, [selectedKey, selectedNode])

  const selectNode = (path: NodePath) => setSelectedKey(pathKey(path))

  const toggleNode = (path: NodePath) => {
    const key = pathKey(path)
    setExpandedKeys((current) => {
      const next = new Set(current)
      if (next.has(key)) next.delete(key)
      else next.add(key)
      return next
    })
  }

  const addNode = (parentPath: NodePath | null, kind: MenuKind) => {
    const nextNode = createEmptyEntry(target, kind)
    let nextNodes: MenuEntry[]
    let nextPath: NodePath

    if (!parentPath) {
      nextPath = [nodes.length]
      nextNodes = [...nodes, nextNode]
    } else {
      const parent = getNodeAt(nodes, parentPath)
      const childIndex = parent?.children?.length ?? 0
      nextPath = [...parentPath, childIndex]
      nextNodes = updateNodeAt(nodes, parentPath, (node) => ({
        ...node,
        kind: "submenu",
        children: [...(node.children ?? []), nextNode],
      }))
    }

    const nextKey = pathKey(nextPath)
    onChange(nextNodes)
    queueMicrotask(() => {
      setSelectedKey(nextKey)
      if (parentPath) {
        setExpandedKeys((current) => new Set(current).add(pathKey(parentPath)))
      }
    })
  }

  const updateNode = (path: NodePath, node: MenuEntry) => {
    onChange(updateNodeAt(nodes, path, () => node))
  }

  const removeNode = (path: NodePath) => {
    const parentPath = path.slice(0, -1)
    onChange(removeNodeAt(nodes, path))
    setSelectedKey(parentPath.length ? pathKey(parentPath) : "")
  }

  return (
    <Field className="min-w-0">
      <FieldLabel>菜单结构</FieldLabel>
      <div className="grid min-h-[360px] min-w-0 gap-3 md:grid-cols-[minmax(240px,0.85fr)_minmax(0,1.4fr)]">
        <div className="min-w-0 overflow-hidden rounded-lg border bg-background">
          <div className="flex h-8 items-center justify-between gap-2 border-b px-2">
            <span className="text-sm font-medium">菜单项</span>
            <AddNodeMenu onAdd={(kind) => addNode(null, kind)} />
          </div>

          {nodes.length ? (
            <ScrollArea className="h-[328px] min-w-0">
              <div className="p-1">
              {nodes.map((node, index) => (
                <TreeNodeRow
                  key={pathKey([index])}
                  node={node}
                  path={[index]}
                  depth={0}
                  selectedKey={selectedKey}
                  expandedKeys={expandedKeys}
                  onSelect={selectNode}
                  onToggle={toggleNode}
                  onAdd={addNode}
                  onRemove={removeNode}
                />
              ))}
              </div>
            </ScrollArea>
          ) : (
            <div className="flex h-[328px] items-center justify-center p-4 text-sm text-muted-foreground">
              暂无菜单项
            </div>
          )}
        </div>

        <div className="min-w-0 rounded-lg border bg-background p-3">
          {selectedNode && selectedPath ? (
            <NodeDetails
              node={selectedNode}
              target={target}
              onChange={(next) => updateNode(selectedPath, next)}
            />
          ) : (
            <div className="flex min-h-48 items-center justify-center text-sm text-muted-foreground">
              选择一个菜单项
            </div>
          )}
        </div>
      </div>
    </Field>
  )
}

type TreeNodeRowProps = {
  node: MenuEntry
  path: NodePath
  depth: number
  selectedKey: string
  expandedKeys: Set<string>
  onSelect: (path: NodePath) => void
  onToggle: (path: NodePath) => void
  onAdd: (parentPath: NodePath | null, kind: MenuKind) => void
  onRemove: (path: NodePath) => void
}

function TreeNodeRow({
  node,
  path,
  depth,
  selectedKey,
  expandedKeys,
  onSelect,
  onToggle,
  onAdd,
  onRemove,
}: TreeNodeRowProps) {
  const key = pathKey(path)
  const selected = selectedKey === key
  const children = node.children ?? []
  const isSubmenu = node.kind === "submenu"
  const expanded = isSubmenu && expandedKeys.has(key)

  return (
    <ContextMenu>
      <ContextMenuTrigger
        className={cn(
          "group flex h-7 min-w-0 items-center gap-1 rounded-md pr-1 text-sm select-none transition-colors",
          selected
            ? "bg-accent text-accent-foreground data-popup-open:bg-accent"
            : "hover:bg-accent/70 data-popup-open:bg-accent/70"
        )}
        style={{ paddingLeft: `${depth * 16 + 4}px` }}
        onClick={() => {
          onSelect(path)
          if (isSubmenu) onToggle(path)
        }}
      >
        <div className="flex size-5 shrink-0 items-center justify-center">
          {isSubmenu ? (
            expanded ? <FolderOpenIcon className="size-4" /> : <FolderIcon className="size-4" />
          ) : (
            <MenuIconPreview icon={node.icon} command={node.command} kind={node.kind} className="size-4" />
          )}
        </div>
        <span className="min-w-0 flex-1 truncate">
          {node.label || (isSubmenu ? "未命名子菜单" : "未命名命令")}
        </span>

        <div
          className="flex shrink-0 items-center gap-0.5 [&_button]:border-0 [&_button]:shadow-none"
          onPointerDown={(event) => event.stopPropagation()}
          onClick={(event) => event.stopPropagation()}
        >
          {isSubmenu ? (
            <AddNodeMenu
              compact
              onAdd={(kind) => onAdd(path, kind)}
              onTriggerClick={(event) => event.stopPropagation()}
            />
          ) : null}
          <Button
            type="button"
            variant="ghost"
            size="icon-xs"
            className="hover:bg-foreground/10"
            aria-label="删除"
            onClick={(event) => {
              event.stopPropagation()
              onRemove(path)
            }}
          >
            <Trash2Icon />
          </Button>
        </div>
      </ContextMenuTrigger>

      <ContextMenuContent>
        <ContextMenuItem onClick={() => onSelect(path)}>
          <PencilIcon />
          编辑
        </ContextMenuItem>
        <ContextMenuSeparator />
        {isSubmenu ? (
          <>
            <ContextMenuItem onClick={() => onAdd(path, "command")}>
              <FileCode2Icon />
              添加命令
            </ContextMenuItem>
            <ContextMenuItem onClick={() => onAdd(path, "submenu")}>
              <FolderTreeIcon />
              添加子菜单
            </ContextMenuItem>
            <ContextMenuSeparator />
          </>
        ) : null}
        <ContextMenuItem onClick={() => onRemove(path)}>
          <Trash2Icon />
          删除
        </ContextMenuItem>
      </ContextMenuContent>

      {expanded ? (
        <div>
          {children.map((child, index) => (
            <TreeNodeRow
              key={pathKey([...path, index])}
              node={child}
              path={[...path, index]}
              depth={depth + 1}
              selectedKey={selectedKey}
              expandedKeys={expandedKeys}
              onSelect={onSelect}
              onToggle={onToggle}
              onAdd={onAdd}
              onRemove={onRemove}
            />
          ))}
        </div>
      ) : null}
    </ContextMenu>
  )
}

type NodeDetailsProps = {
  node: MenuEntry
  target: TargetType
  onChange: (node: MenuEntry) => void
}

function NodeDetails({ node, target, onChange }: NodeDetailsProps) {
  return (
    <div className="flex min-w-0 flex-col gap-3">
      <div className="flex min-w-0 items-center gap-2">
        <div className="flex size-8 shrink-0 items-center justify-center rounded-md bg-muted">
          <MenuIconPreview icon={node.icon} command={node.command} kind={node.kind} className="size-5" />
        </div>
        <span className="truncate font-medium">
          {node.label || (node.kind === "submenu" ? "未命名子菜单" : "未命名命令")}
        </span>
      </div>

      <Field className="min-w-0">
        <FieldLabel>名称</FieldLabel>
        <Input value={node.label} onChange={(event) => onChange({ ...node, label: event.target.value })} />
      </Field>

      <IconField
        icon={node.icon}
        command={node.command}
        kind={node.kind as MenuKind}
        onChange={(icon) => onChange({ ...node, icon })}
      />

	  <label className="flex min-h-9 cursor-pointer items-center justify-between gap-3 rounded-lg border px-3 text-sm transition-colors hover:bg-muted/30">
		<span>仅按住 Shift 时显示</span>
		<Switch
		  size="sm"
		  checked={node.shiftOnly}
		  onCheckedChange={(shiftOnly) => onChange({ ...node, shiftOnly })}
		/>
	  </label>

      {node.kind === "command" ? (
        <CommandField
          value={node.command}
          target={target}
          onChange={(command) => onChange({ ...node, command, commandTemplate: command })}
        />
      ) : null}
    </div>
  )
}

type AddNodeMenuProps = {
  onAdd: (kind: MenuKind) => void
  label?: string
  compact?: boolean
  onTriggerClick?: (event: React.MouseEvent<HTMLButtonElement>) => void
}

function AddNodeMenu({ onAdd, label, compact = false, onTriggerClick }: AddNodeMenuProps) {
  return (
    <DropdownMenu>
      <DropdownMenuTrigger
        render={
          <Button
            type="button"
            variant="ghost"
            size={compact ? "icon-xs" : label ? "sm" : "icon-sm"}
            className={compact ? "hover:bg-foreground/10 aria-expanded:bg-foreground/10 data-popup-open:bg-foreground/10" : undefined}
            aria-label={label ?? "添加子项"}
            onClick={onTriggerClick}
          >
            <PlusIcon />
            {!compact && label ? label : null}
          </Button>
        }
      />
      <DropdownMenuContent align="end" className="w-auto min-w-32">
        <DropdownMenuItem onClick={() => onAdd("command")}>
          <FileCode2Icon />
          命令
        </DropdownMenuItem>
        <DropdownMenuItem onClick={() => onAdd("submenu")}>
          <FolderTreeIcon />
          子菜单
        </DropdownMenuItem>
      </DropdownMenuContent>
    </DropdownMenu>
  )
}

function pathKey(path: NodePath) {
  return path.join(".")
}

function pathFromKey(key: string): NodePath {
  return key.split(".").map(Number)
}

function getNodeAt(nodes: MenuEntry[], path: NodePath): MenuEntry | null {
  let current: MenuEntry | undefined
  let list = nodes
  for (const index of path) {
    current = list[index]
    if (!current) return null
    list = current.children ?? []
  }
  return current ?? null
}

function updateNodeAt(
  nodes: MenuEntry[],
  path: NodePath,
  update: (node: MenuEntry) => MenuEntry,
): MenuEntry[] {
  const [index, ...rest] = path
  if (index === undefined || !nodes[index]) return nodes

  return nodes.map((node, nodeIndex) => {
    if (nodeIndex !== index) return node
    if (!rest.length) return update(node)
    return { ...node, children: updateNodeAt(node.children ?? [], rest, update) }
  })
}

function removeNodeAt(nodes: MenuEntry[], path: NodePath): MenuEntry[] {
  const [index, ...rest] = path
  if (index === undefined || !nodes[index]) return nodes
  if (!rest.length) return nodes.filter((_, nodeIndex) => nodeIndex !== index)

  return nodes.map((node, nodeIndex) =>
    nodeIndex === index
      ? { ...node, children: removeNodeAt(node.children ?? [], rest) }
      : node,
  )
}


