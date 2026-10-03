import { useCallback, useEffect, useRef, useState } from "react"
import {
  LoaderCircleIcon,
  MoreHorizontalIcon,
  PencilIcon,
  PlusIcon,
  Trash2Icon,
} from "lucide-react"

import {
  ScheduledTaskTemplateService,
  type ScheduledTaskTemplate,
} from "../../../bindings/win-admin"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogDescription,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import {
  DropdownMenu,
  DropdownMenuContent,
  DropdownMenuGroup,
  DropdownMenuItem,
  DropdownMenuTrigger,
} from "@/components/ui/dropdown-menu"
import { Empty, EmptyHeader, EmptyTitle } from "@/components/ui/empty"
import { ScrollArea } from "@/components/ui/scroll-area"
import { TruncatedText } from "@/components/ui/truncated-text"
import { showError } from "@/lib/error-toast"

type ScheduledTaskTemplatesDialogProps = {
  open: boolean
  refreshKey: number
  onOpenChange: (open: boolean) => void
  onCreate: () => void
  onUse: (template: ScheduledTaskTemplate) => void
  onEdit: (template: ScheduledTaskTemplate) => void
}

export function ScheduledTaskTemplatesDialog({
  open,
  refreshKey,
  onOpenChange,
  onCreate,
  onUse,
  onEdit,
}: ScheduledTaskTemplatesDialogProps) {
  const [templates, setTemplates] = useState<ScheduledTaskTemplate[]>([])
  const [loading, setLoading] = useState(false)
  const [deleteTemplate, setDeleteTemplate] = useState<ScheduledTaskTemplate | null>(null)
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [saving, setSaving] = useState(false)
  const hasLoadedRef = useRef(false)

  const load = useCallback(async (silent = false) => {
    if (!silent) setLoading(true)
    try {
      setTemplates((await ScheduledTaskTemplateService.List()) ?? [])
      hasLoadedRef.current = true
    } catch (reason) {
      showError(reason)
    } finally {
      if (!silent) setLoading(false)
    }
  }, [])

  useEffect(() => {
    if (open) void load(hasLoadedRef.current)
  }, [load, open, refreshKey])

  const confirmDelete = async () => {
    if (!deleteTemplate) return
    setSaving(true)
    try {
      await ScheduledTaskTemplateService.Delete(deleteTemplate.id)
      setDeleteOpen(false)
      await load(true)
    } catch (reason) {
      showError(reason, "无法删除模板")
    } finally {
      setSaving(false)
    }
  }

  return (
    <>
      <Dialog open={open} onOpenChange={onOpenChange}>
        <DialogContent className="flex h-[min(620px,calc(100vh-2rem))] !w-[min(92vw,44rem)] min-w-0 !max-w-none flex-col overflow-hidden">
          <Button
            variant="ghost"
            size="icon-sm"
            className="absolute top-2 right-11"
            onClick={onCreate}
            aria-label="新建模板"
          >
            <PlusIcon />
          </Button>
          <DialogHeader>
            <DialogTitle>计划任务模板</DialogTitle>
          </DialogHeader>

          <ScrollArea className="-mr-4 min-h-0 flex-1 pr-4">
            {loading ? (
              <div className="flex min-h-full items-center justify-center" aria-label="加载中">
                <LoaderCircleIcon className="size-5 animate-spin text-muted-foreground" />
              </div>
            ) : templates.length ? (
              <div className="overflow-hidden rounded-lg border">
                {templates.map((template) => {
                  const actionSummary = [
                    template.task.action.command,
                    template.task.action.arguments,
                  ].filter(Boolean).join(" ")
                  return (
                  <div key={template.id} className="flex min-h-12 min-w-0 items-center gap-2 border-b px-3 last:border-b-0 hover:bg-muted/30">
                    <button
                      type="button"
                      className="flex min-w-0 flex-1 cursor-pointer flex-col gap-1 text-left"
                      onClick={() => {
                        onOpenChange(false)
                        onUse(template)
                      }}
                    >
                      <TruncatedText value={template.name} className="w-full text-sm font-medium" />
                      <TruncatedText value={actionSummary || "未配置操作"} className="w-full text-xs text-muted-foreground" />
                    </button>
                    <DropdownMenu>
                      <DropdownMenuTrigger render={<Button variant="ghost" size="icon-sm" aria-label="更多操作" />}>
                        <MoreHorizontalIcon />
                      </DropdownMenuTrigger>
                      <DropdownMenuContent align="end" className="w-32">
                        <DropdownMenuGroup>
                          <DropdownMenuItem onClick={() => onEdit(template)}>
                            <PencilIcon />
                            编辑
                          </DropdownMenuItem>
                          <DropdownMenuItem onClick={() => {
                            setDeleteTemplate(template)
                            setDeleteOpen(true)
                          }}>
                            <Trash2Icon />
                            删除
                          </DropdownMenuItem>
                        </DropdownMenuGroup>
                      </DropdownMenuContent>
                    </DropdownMenu>
                  </div>
                  )
                })}
              </div>
            ) : (
              <Empty className="min-h-48 border">
                <EmptyHeader>
                  <EmptyTitle>还没有模板</EmptyTitle>
                </EmptyHeader>
              </Empty>
            )}
          </ScrollArea>
        </DialogContent>
      </Dialog>

      <Dialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        onOpenChangeComplete={(next) => !next && setDeleteTemplate(null)}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>删除模板</DialogTitle>
            <DialogDescription>删除“{deleteTemplate?.name}”？</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleteOpen(false)} disabled={saving}>取消</Button>
            <Button onClick={() => void confirmDelete()} disabled={saving}>删除</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </>
  )
}

