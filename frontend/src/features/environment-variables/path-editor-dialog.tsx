import { useEffect, useRef, useState } from "react"
import {
  ArrowDownIcon,
  ArrowUpIcon,
  ArrowUpToLineIcon,
  LoaderCircleIcon,
  PlusIcon,
  SearchIcon,
  TriangleAlertIcon,
  Trash2Icon,
} from "lucide-react"

import {
  EnvironmentVariableService,
  type EnvironmentPathEntry,
} from "../../../bindings/win-admin"
import { Alert, AlertDescription, AlertTitle } from "@/components/ui/alert"
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
import { InputGroup, InputGroupAddon, InputGroupInput } from "@/components/ui/input-group"
import { Empty, EmptyHeader, EmptyTitle } from "@/components/ui/empty"
import { ScrollArea } from "@/components/ui/scroll-area"
import { Separator } from "@/components/ui/separator"
import { showError } from "@/lib/error-toast"
import { cn } from "@/lib/utils"

type PathEditorDialogProps = {
  open: boolean
  scope: "system" | "user"
  onOpenChange: (open: boolean) => void
  onSaved: () => Promise<void>
}

type PathEntry = EnvironmentPathEntry & { id: number; validated: boolean }

function isMissingEntry(entry: PathEntry) {
  return entry.validated && entry.value.trim() !== "" && !entry.exists
}

export function PathEditorDialog({ open, scope, onOpenChange, onSaved }: PathEditorDialogProps) {
  const [entries, setEntries] = useState<PathEntry[]>([])
  const [selectedId, setSelectedId] = useState<number | null>(null)
  const [loading, setLoading] = useState(false)
  const [saving, setSaving] = useState(false)
  const [query, setQuery] = useState("")
  const nextEntryId = useRef(0)
  const pendingFocusId = useRef<number | null>(null)
  const pendingScrollToTop = useRef(false)
  const inputRefs = useRef(new Map<number, HTMLInputElement>())
  const keyword = query.trim().toLowerCase()
  const visibleEntries = entries.map((entry, index) => ({ entry, index })).filter(({ entry }) => (
    !keyword || `${entry.value}\n${entry.validated ? entry.expanded : ""}`.toLowerCase().includes(keyword)
  ))
  const missingEntries = visibleEntries.map(({ entry }) => entry).filter(isMissingEntry)
  const duplicateEntries = visibleEntries.map(({ entry }) => entry).filter((entry) => entry.validated && entry.duplicate)
  const selectedIndex = selectedId === null ? -1 : entries.findIndex((entry) => entry.id === selectedId)

  const canMoveSelected = (offset: -1 | 1) => {
    if (selectedIndex < 0) return false
    const target = selectedIndex + offset
    return target >= 0 && target < entries.length
  }

  useEffect(() => {
    if (!open) return
    setQuery("")
    setLoading(true)
    void EnvironmentVariableService.GetPath(scope)
      .then((items) => {
        const next = items ?? []
        setEntries(next.map((entry) => ({
          ...entry,
          id: nextEntryId.current++,
          validated: true,
        })))
        setSelectedId(null)
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
          if (!active) return
          const analyzed = items ?? []
          setEntries((current) => {
            if (analyzed.length !== current.length) return current
            return current.map((entry, index) => ({
              ...entry,
              ...analyzed[index],
              validated: true,
            }))
          })
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
    if (!canMoveSelected(offset)) return
    const target = selectedIndex + offset
    setEntries((current) => {
      const next = [...current]
      ;[next[selectedIndex], next[target]] = [next[target], next[selectedIndex]]
      return next
    })
  }

  const moveSelectedToTop = () => {
    if (selectedIndex <= 0) return
    pendingScrollToTop.current = true
    setEntries((current) => {
      const next = [...current]
      const [entry] = next.splice(selectedIndex, 1)
      next.unshift(entry)
      return next
    })
  }

  const removeEntry = (id: number) => {
    setEntries((current) => current.filter((entry) => entry.id !== id))
    if (id === selectedId) setSelectedId(null)
  }

  const locateEntry = (id: number) => {
    setSelectedId(id)
    const input = inputRefs.current.get(id)
    input?.focus({ preventScroll: true })
    input?.scrollIntoView({ block: "nearest" })
  }

  const addEntry = () => {
    const id = nextEntryId.current++
    setQuery("")
    pendingFocusId.current = id
    setEntries((current) => [...current, { id, value: "", expanded: "", exists: false, duplicate: false, validated: false }])
    setSelectedId(id)
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
          <InputGroup className="shrink-0">
            <InputGroupInput
              value={query}
              onChange={(event) => setQuery(event.target.value)}
              placeholder="搜索路径"
              aria-label="搜索 PATH"
            />
            <InputGroupAddon><SearchIcon /></InputGroupAddon>
          </InputGroup>
          <ScrollArea className="-mr-4 min-h-0 flex-1 pr-4">
            {loading ? (
              <div className="flex min-h-full items-center justify-center" aria-label="加载中">
                <LoaderCircleIcon className="size-5 animate-spin text-muted-foreground" />
              </div>
            ) : (
              <div className="flex min-h-full flex-col gap-2 p-1">
                {[
                  { title: `发现 ${missingEntries.length} 个无效路径`, items: missingEntries },
                  { title: `发现 ${duplicateEntries.length} 个重复项`, items: duplicateEntries },
                ].filter(({ items }) => items.length > 0).map(({ title, items }) => (
                  <Alert key={title} className="gap-y-1">
                    <TriangleAlertIcon />
                    <AlertTitle>{title}</AlertTitle>
                    <AlertDescription className="flex flex-col gap-0">
                      {items.map((entry) => (
                        <div key={entry.id} className="flex min-w-0 items-center gap-1.5">
                          <span className="min-w-0 flex-1 truncate font-mono text-xs">{entry.value}</span>
                          <Button variant="ghost" size="xs" className="h-5" onClick={() => locateEntry(entry.id)}>定位</Button>
                          <Button variant="ghost" size="xs" className="h-5" onClick={() => removeEntry(entry.id)}>删除</Button>
                        </div>
                      ))}
                    </AlertDescription>
                  </Alert>
                ))}
                {missingEntries.length > 0 || duplicateEntries.length > 0 ? <Separator /> : null}
                {visibleEntries.map(({ entry, index }) => (
                  <div
                    key={entry.id}
                    className={cn(
                      "flex min-w-0 items-center gap-1.5 rounded-lg p-1 transition-colors",
                      selectedId === entry.id && "bg-muted/50"
                    )}
                  >
                    <div className="relative min-w-0 flex-1">
                      <Input
                        ref={(input) => {
                          if (input) inputRefs.current.set(entry.id, input)
                          else inputRefs.current.delete(entry.id)
                          if (input && pendingFocusId.current === entry.id) {
                            pendingFocusId.current = null
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
                        onFocus={() => setSelectedId(entry.id)}
                        className={cn("min-w-0 font-mono text-xs", {
                          "pr-16": isMissingEntry(entry) || (entry.validated && entry.duplicate),
                          "pr-28": isMissingEntry(entry) && entry.validated && entry.duplicate,
                        })}
                        aria-label={`PATH 项 ${index + 1}`}
                      />
                      <div className="pointer-events-none absolute top-1/2 right-2 flex -translate-y-1/2 gap-1">
                        {isMissingEntry(entry) ? <Badge variant="destructive">不存在</Badge> : null}
                        {entry.validated && entry.duplicate ? <Badge variant="secondary">重复</Badge> : null}
                      </div>
                    </div>
                    <Button variant="ghost" size="icon-sm" onClick={() => removeEntry(entry.id)} aria-label="删除 PATH 项">
                      <Trash2Icon />
                    </Button>
                  </div>
                ))}
                {visibleEntries.length === 0 ? (
                  <Empty className="flex-1">
                    <EmptyHeader><EmptyTitle className="text-muted-foreground">{keyword ? "没有匹配的路径" : "没有路径"}</EmptyTitle></EmptyHeader>
                  </Empty>
                ) : null}
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
              disabled={selectedIndex <= 0 || saving || loading}
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
