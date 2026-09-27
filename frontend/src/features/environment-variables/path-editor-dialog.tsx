import { useEffect, useRef, useState } from "react"
import {
  ArrowDownIcon,
  ArrowUpIcon,
  ArrowUpToLineIcon,
  LoaderCircleIcon,
  PlusIcon,
  Trash2Icon,
} from "lucide-react"

import {
  EnvironmentVariableService,
  type EnvironmentPathEntry,
} from "../../../bindings/win-admin"
import { Badge } from "@/components/ui/badge"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Input } from "@/components/ui/input"
import { ScrollArea } from "@/components/ui/scroll-area"
import { showError } from "@/lib/error-toast"

type PathEditorDialogProps = {
  open: boolean
  scope: "system" | "user"
  onOpenChange: (open: boolean) => void
  onSaved: () => Promise<void>
}

type PathEntry = EnvironmentPathEntry & { validated: boolean }

export function PathEditorDialog({ open, scope, onOpenChange, onSaved }: PathEditorDialogProps) {
  const [entries, setEntries] = useState<PathEntry[]>([])
  const [selectedIndex, setSelectedIndex] = useState<number | null>(null)
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const pendingFocusIndex = useRef<number | null>(null)
  const pendingScrollToTop = useRef(false)

  const canMoveSelected = (offset: -1 | 1) => {
    return selectedIndex !== null && selectedIndex + offset >= 0 && selectedIndex + offset < entries.length
  }

  useEffect(() => {
    if (!open) return
    setLoading(true)
    void EnvironmentVariableService.GetPath(scope)
      .then((items) => {
        const next = items ?? []
        setEntries(next.map((entry) => ({ ...entry, validated: true })))
        setSelectedIndex(null)
      })
      .catch((reason) => showError(reason, "无法读取 PATH"))
      .finally(() => setLoading(false))
  }, [open, scope])

  useEffect(() => {
    if (!open || loading) return
    let active = true
    const timer = window.setTimeout(() => {
      void EnvironmentVariableService.AnalyzePath(scope, entries.map((entry) => entry.value))
        .then((items) => {
          if (active) setEntries((items ?? []).map((entry) => ({ ...entry, validated: true })))
        })
        .catch((reason) => {
          if (active) showError(reason, "PATH 检查失败")
        })
    }, 250)
    return () => {
      active = false
      window.clearTimeout(timer)
    }
  }, [entries.map((entry) => entry.value).join("\u0000"), loading, open, scope])

  const updateEntry = (index: number, value: string) => {
    setEntries((current) => current.map((entry, itemIndex) => (
      itemIndex === index ? { ...entry, value, validated: false } : entry
    )))
  }

  const moveSelected = (offset: -1 | 1) => {
    if (!canMoveSelected(offset) || selectedIndex === null) return
    const target = selectedIndex + offset
    setEntries((current) => {
      const next = [...current]
      ;[next[selectedIndex], next[target]] = [next[target], next[selectedIndex]]
      return next
    })
    setSelectedIndex(target)
  }

  const moveSelectedToTop = () => {
    if (selectedIndex === null || selectedIndex <= 0) return
    pendingScrollToTop.current = true
    setEntries((current) => {
      const next = [...current]
      const [entry] = next.splice(selectedIndex, 1)
      next.unshift(entry)
      return next
    })
    setSelectedIndex(0)
  }

  const removeEntry = (index: number) => {
    setEntries((current) => current.filter((_, itemIndex) => itemIndex !== index))
    setSelectedIndex((current) => {
      if (current === null) return null
      if (current === index) return entries.length <= 1 ? null : Math.min(index, entries.length - 2)
      return current > index ? current - 1 : current
    })
  }

  const addEntry = () => {
    pendingFocusIndex.current = entries.length
    setEntries((current) => [...current, { value: "", expanded: "", exists: false, validated: false }])
    setSelectedIndex(entries.length)
  }

  const save = async () => {
    setSaving(true)
    try {
      await EnvironmentVariableService.SavePath({ scope, entries: entries.map((entry) => entry.value) })
      await onSaved()
      onOpenChange(false)
    } catch (reason) {
      showError(reason, "无法保存 PATH")
    } finally {
      setSaving(false)
    }
  }

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent initialFocus={false} className="flex h-[min(680px,calc(100vh-2rem))] !w-[min(92vw,52rem)] min-w-0 !max-w-none flex-col overflow-hidden">
        <DialogHeader>
          <DialogTitle>编辑 PATH</DialogTitle>
        </DialogHeader>

        <div className="flex min-h-0 flex-1 flex-col gap-2">
          <ScrollArea className="min-h-0 flex-1 pr-3">
            {loading ? (
              <div className="flex min-h-full items-center justify-center" aria-label="加载中">
                <LoaderCircleIcon className="size-5 animate-spin text-muted-foreground" />
              </div>
            ) : (
              <div className="flex flex-col gap-1">
                {entries.map((entry, index) => (
                  <div
                    key={index}
                    className={`flex min-w-0 items-center gap-1.5 rounded-lg p-1 transition-colors ${selectedIndex === index ? "bg-muted/50" : ""}`}
                  >
                    <div className="relative min-w-0 flex-1">
                      <Input
                        ref={(input) => {
                          if (input && pendingFocusIndex.current === index) {
                            pendingFocusIndex.current = null
                            input.focus()
                          }
                          if (input && index === 0 && pendingScrollToTop.current) {
                            pendingScrollToTop.current = false
                            const viewport = input.closest<HTMLElement>('[data-slot="scroll-area-viewport"]')
                            if (viewport) viewport.scrollTop = 0
                          }
                        }}
                        value={entry.value}
                        onChange={(event) => updateEntry(index, event.target.value)}
                        onFocus={() => setSelectedIndex(index)}
                        className="min-w-0 pr-16 font-mono text-xs"
                        aria-label={`PATH 项 ${index + 1}`}
                      />
                      {entry.value.trim() && entry.validated && !entry.exists ? (
                        <Badge variant="destructive" className="pointer-events-none absolute top-1/2 right-2 -translate-y-1/2">不存在</Badge>
                      ) : null}
                    </div>
                    <Button variant="ghost" size="icon-sm" onClick={() => removeEntry(index)} aria-label="删除 PATH 项">
                      <Trash2Icon />
                    </Button>
                  </div>
                ))}
              </div>
            )}
          </ScrollArea>
        </div>

        <DialogFooter>
          <div className="mr-auto flex items-center gap-1.5">
            <Button variant="outline" size="sm" onClick={addEntry} disabled={saving || loading}>
              <PlusIcon data-icon="inline-start" />
              添加
            </Button>
            <Button
              variant="ghost"
              size="icon-sm"
              onClick={moveSelectedToTop}
              disabled={selectedIndex === null || selectedIndex <= 0 || saving || loading}
              aria-label="置顶选中项"
            >
              <ArrowUpToLineIcon />
            </Button>
            <Button
              variant="ghost"
              size="icon-sm"
              onClick={() => moveSelected(-1)}
              disabled={!canMoveSelected(-1) || saving || loading}
              aria-label="上移选中项"
            >
              <ArrowUpIcon />
            </Button>
            <Button
              variant="ghost"
              size="icon-sm"
              onClick={() => moveSelected(1)}
              disabled={!canMoveSelected(1) || saving || loading}
              aria-label="下移选中项"
            >
              <ArrowDownIcon />
            </Button>
          </div>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={saving}>取消</Button>
          <Button onClick={() => void save()} disabled={saving || loading}>保存</Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}
