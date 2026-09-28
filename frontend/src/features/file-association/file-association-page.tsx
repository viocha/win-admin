import { useCallback, useEffect, useState } from "react"
import { CopyIcon, ExternalLinkIcon, LoaderCircleIcon, PencilIcon, PlusIcon, RefreshCwIcon, Trash2Icon } from "lucide-react"

import { FileAssociationService, type FileAssociation } from "../../../bindings/win-admin"
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
import { Empty, EmptyHeader, EmptyTitle } from "@/components/ui/empty"
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import { ScrollArea } from "@/components/ui/scroll-area"
import { SidebarTrigger } from "@/components/ui/sidebar"
import { Textarea } from "@/components/ui/textarea"
import { TruncatedText } from "@/components/ui/truncated-text"
import { showError } from "@/lib/error-toast"

const displayExtension = (value: string) => value.replace(/^\./, "")

export function FileAssociationPage({ active }: { active: boolean }) {
  const [items, setItems] = useState<FileAssociation[]>([])
  const [loaded, setLoaded] = useState(false)
  const [loading, setLoading] = useState(true)
  const [editorOpen, setEditorOpen] = useState(false)
  const [editingExtension, setEditingExtension] = useState("")
  const [extension, setExtension] = useState("")
  const [command, setCommand] = useState("")
  const [saving, setSaving] = useState(false)
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [deleteItem, setDeleteItem] = useState<FileAssociation | null>(null)
  const [deleting, setDeleting] = useState(false)

  const load = useCallback(async () => {
    setLoading(true)
    try {
      setItems((await FileAssociationService.List()) ?? [])
      setLoaded(true)
    } catch (reason) {
      showError(reason, "无法读取打开方式")
    } finally {
      setLoading(false)
    }
  }, [])

  useEffect(() => {
    if (!active) return
    void load()
    const refreshOnFocus = () => { void load() }
    window.addEventListener("focus", refreshOnFocus)
    return () => window.removeEventListener("focus", refreshOnFocus)
  }, [active, load])

  const edit = (item?: FileAssociation) => {
    setEditingExtension(item?.extension ?? "")
    setExtension(item ? displayExtension(item.extension) : "")
    setCommand(item?.command ?? "")
    setEditorOpen(true)
  }

  const save = async () => {
    if (saving) return
    setSaving(true)
    try {
      if (editingExtension) {
        await FileAssociationService.Rename(editingExtension, extension, command)
      } else {
        await FileAssociationService.Save(extension, command)
      }
      setEditorOpen(false)
      await load()
    } catch (reason) {
      showError(reason, "无法保存打开方式")
    } finally {
      setSaving(false)
    }
  }

  const remove = async () => {
    if (!deleteItem || deleting) return
    setDeleting(true)
    try {
      await FileAssociationService.Delete(deleteItem.extension)
      setDeleteOpen(false)
      await load()
    } catch (reason) {
      showError(reason, "无法删除打开方式")
    } finally {
      setDeleting(false)
    }
  }

  const openSettings = async () => {
    try {
      await FileAssociationService.OpenDefaultApps()
    } catch (reason) {
      showError(reason, "无法打开默认应用设置")
    }
  }

  return (
    <div className="flex h-full min-h-0 w-full flex-col p-4 lg:p-6">
      <div className="flex shrink-0 flex-wrap items-center justify-between gap-3">
        <div className="flex min-w-0 items-center gap-2">
          <SidebarTrigger className="-ml-1 shrink-0" />
          <h1 className="text-xl font-semibold tracking-tight">打开方式</h1>
        </div>
        <div className="flex items-center gap-2">
          <Button variant="outline" onClick={() => void load()} disabled={loading} aria-label="刷新">
            <RefreshCwIcon data-icon="inline-start" className={loading ? "animate-spin" : ""} />
            刷新
          </Button>
          <Button variant="outline" onClick={() => void openSettings()}>
            <ExternalLinkIcon data-icon="inline-start" />
            默认应用设置
          </Button>
          <Button onClick={() => edit()}>
            <PlusIcon data-icon="inline-start" />
            添加打开方式
          </Button>
        </div>
      </div>

      <p className="mt-3 shrink-0 text-sm text-muted-foreground">
        仅显示本工具添加的命令；保存后需在 Windows 默认应用设置中选择“Win Admin”，双击才会生效。
      </p>

      <div className="mt-4 min-h-0 flex-1">
        {!loaded && loading ? (
          <div className="flex h-full items-center justify-center">
            <LoaderCircleIcon className="size-5 animate-spin text-muted-foreground" />
          </div>
        ) : items.length === 0 ? (
          <Empty className="h-full border">
            <EmptyHeader>
              <EmptyTitle>暂无打开方式</EmptyTitle>
            </EmptyHeader>
          </Empty>
        ) : (
          <ScrollArea className="h-full min-w-0">
            <div className="overflow-hidden rounded-lg border bg-card">
              <div className="hidden h-8 grid-cols-[7rem_minmax(0,1fr)_7rem] items-center gap-3 border-b bg-muted/20 px-3 text-xs text-muted-foreground md:grid">
                <div>扩展名</div>
                <div>打开命令</div>
                <div>状态</div>
              </div>
              {items.map((item) => (
                <ContextMenu key={item.extension}>
                  <ContextMenuTrigger className="flex min-w-0 items-stretch border-b last:border-b-0 hover:bg-muted/30">
                    <button
                      type="button"
                      className="grid min-h-10 min-w-0 flex-1 cursor-pointer grid-cols-[5rem_minmax(0,1fr)_auto] items-center gap-3 px-3 text-left md:grid-cols-[7rem_minmax(0,1fr)_7rem]"
                      onClick={() => edit(item)}
                    >
                      <span className="font-mono text-sm font-medium">{displayExtension(item.extension)}</span>
                      <TruncatedText value={item.command} className="min-w-0 font-mono text-xs text-muted-foreground" />
                      <Badge variant={item.active ? "secondary" : "outline"} className="justify-self-start">
                        {item.active ? "已设默认" : "待选择"}
                      </Badge>
                    </button>
                  </ContextMenuTrigger>
                  <ContextMenuContent className="min-w-max">
                    <ContextMenuItem onClick={() => edit(item)}>
                      <PencilIcon />
                      编辑
                    </ContextMenuItem>
                    <ContextMenuSeparator />
                    <ContextMenuItem
                      className="whitespace-nowrap"
                      onClick={() => navigator.clipboard.writeText(item.registryPath)}
                    >
                      <CopyIcon />
                      复制注册表路径
                    </ContextMenuItem>
                    <ContextMenuSeparator />
                    <ContextMenuItem onClick={() => { setDeleteItem(item); setDeleteOpen(true) }}>
                      <Trash2Icon />
                      删除
                    </ContextMenuItem>
                  </ContextMenuContent>
                </ContextMenu>
              ))}
            </div>
          </ScrollArea>
        )}
      </div>

      <Dialog open={editorOpen} onOpenChange={setEditorOpen}>
        <DialogContent className="sm:max-w-lg">
          <DialogHeader>
            <DialogTitle>{editingExtension ? `编辑 ${displayExtension(editingExtension)}` : "添加打开方式"}</DialogTitle>
            <DialogDescription>%1 表示文件路径，%u 表示 file URL；都省略时将路径作为最后一个参数传入。</DialogDescription>
          </DialogHeader>
          <FieldGroup>
            <Field>
              <FieldLabel htmlFor="association-extension">扩展名</FieldLabel>
              <Input
                id="association-extension"
                value={extension}
                onChange={(event) => setExtension(event.target.value)}
                placeholder="ts"
              />
            </Field>
            <Field>
              <FieldLabel htmlFor="association-command">打开命令</FieldLabel>
              <Textarea
                id="association-command"
                rows={3}
                value={command}
                onChange={(event) => setCommand(event.target.value)}
                placeholder={'bun run 或 msedge --app="%u"'}
                className="max-h-40 resize-none font-mono text-xs"
              />
            </Field>
          </FieldGroup>
          <DialogFooter>
            <Button variant="outline" onClick={() => setEditorOpen(false)}>取消</Button>
            <Button onClick={() => void save()} disabled={!extension.trim() || !command.trim() || saving}>保存</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <Dialog open={deleteOpen} onOpenChange={setDeleteOpen}>
        <DialogContent>
          <DialogHeader>
            <DialogTitle>删除 {deleteItem ? displayExtension(deleteItem.extension) : ""} 的打开方式</DialogTitle>
            <DialogDescription>
              {deleteItem?.active ? "删除后 Windows 可能需要重新选择默认应用。" : "将移除该扩展名的自定义命令。"}
            </DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleteOpen(false)}>取消</Button>
            <Button variant="destructive" onClick={() => void remove()} disabled={deleting}>删除</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
