import { useEffect, useMemo, useState } from "react"
import {
  ArrowDownIcon,
  ArrowUpIcon,
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

export function PathEditorDialog({ open, scope, onOpenChange, onSaved }: PathEditorDialogProps) {
  const [entries, setEntries] = useState<EnvironmentPathEntry[]>([])
  const [selectedIndex, setSelectedIndex] = useState<number | null>(null)
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)

  const displayEntries = useMemo(() => (
    entries
      .map((entry, index) => ({ entry, index }))
      .toSorted((left, right) => {
        if (left.entry.exists !== right.entry.exists) return left.entry.exists ? 1 : -1
        return left.index - right.index
      })
  ), [entries])

  const selectedDisplayIndex = selectedIndex === null
    ? -1
    : displayEntries.findIndex((item) => item.index === selectedIndex)

  const canMoveSelected = (offset: -1 | 1) => {
    if (selectedDisplayIndex < 0 || selectedIndex === null) return false
    const target = displayEntries[selectedDisplayIndex + offset]
    return !!target && target.entry.exists === entries[selectedIndex]?.exists
  }

  useEffect(() => {
    if (!open) return
    setLoading(true)
    void EnvironmentVariableService.GetPath(scope)
      .then((items) => {
        const next = items ?? []
        setEntries(next)
        setSelectedIndex(null)
      })
      .catch((reason) => showError(reason, "无法读取 PATH"))
      .finally(() => setLoading(false))
  }, [open, scope])

  useEffect(() => {
    if (!open || loading) return
    const timer = window.setTimeout(() => {
      void EnvironmentVariableService.AnalyzePath(scope, entries.map((entry) => entry.value))
        .then((items) => setEntries(items ?? []))
        .catch((reason) => showError(reason, "PATH 检查失败"))
    }, 250)
    return () => window.clearTimeout(timer)
  }, [entries.map((entry) => entry.value).join("\u0000"), loading, open, scope])

  const updateEntry = (index: number, value: string) => {
    setEntries((current) => current.map((entry, itemIndex) => (
      itemIndex === index ? { ...entry, value } : entry
    )))
  }

  const moveSelected = (offset: -1 | 1) => {
    if (selectedIndex === null || selectedDisplayIndex < 0) return
    const targetItem = displayEntries[selectedDisplayIndex + offset]
    if (!targetItem || targetItem.entry.exists !== entries[selectedIndex]?.exists) return
    const target = targetItem.index
    setEntries((current) => {
      const next = [...current]
      ;[next[selectedIndex], next[target]] = [next[target], next[selectedIndex]]
      return next
    })
    setSelectedIndex(target)
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
    setEntries((current) => [...current, { value: "", expanded: "", exists: false }])
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
                {displayEntries.map(({ entry, index }) => (
                  <div
                    key={index}
                    className={`flex min-w-0 items-center gap-1.5 rounded-lg p-1 transition-colors ${selectedIndex === index ? "bg-muted/50" : ""}`}
                  >
                    <Input
                      value={entry.value}
                      onChange={(event) => updateEntry(index, event.target.value)}
                      onFocus={() => setSelectedIndex(index)}
                      className="min-w-0 flex-1 font-mono text-xs"
                      aria-label={`PATH 项 ${index + 1}`}
                    />
                    {entry.value.trim() && !entry.exists ? <Badge variant="destructive">不存在</Badge> : null}
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

