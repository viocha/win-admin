import { useCallback, useEffect, useMemo, useRef, useState } from "react"
import {
  LoaderCircleIcon,
  PlusIcon,
  RefreshCwIcon,
  SearchIcon,
  Trash2Icon,
} from "lucide-react"

import {
  EnvironmentVariableService,
  type EnvironmentVariable,
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
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuTrigger,
} from "@/components/ui/context-menu"
import { Empty, EmptyHeader, EmptyTitle } from "@/components/ui/empty"
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from "@/components/ui/input-group"
import { ScrollArea } from "@/components/ui/scroll-area"
import { SidebarTrigger } from "@/components/ui/sidebar"
import { Tabs, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Textarea } from "@/components/ui/textarea"
import { TruncatedText } from "@/components/ui/truncated-text"
import { showError } from "@/lib/error-toast"
import { PathEditorDialog } from "./path-editor-dialog"

type Scope = "system" | "user"

export function EnvironmentVariablesPage({ active }: { active: boolean }) {
  const [scope, setScope] = useState<Scope>("system")
  const [variables, setVariables] = useState<EnvironmentVariable[]>([])
  const [loading, setLoading] = useState(true)
  const [query, setQuery] = useState("")
  const [editorVariable, setEditorVariable] = useState<EnvironmentVariable | null>(null)
  const [previousName, setPreviousName] = useState("")
  const [editorOpen, setEditorOpen] = useState(false)
  const [deleteVariable, setDeleteVariable] = useState<EnvironmentVariable | null>(null)
  const [deleteOpen, setDeleteOpen] = useState(false)
  const [pathOpen, setPathOpen] = useState(false)
  const [saving, setSaving] = useState(false)
  const cacheRef = useRef(new Map<Scope, EnvironmentVariable[]>())

  const load = useCallback(async (nextScope: Scope, silent = false) => {
    if (!silent) setLoading(true)
    try {
      const items = (await EnvironmentVariableService.List(nextScope)) ?? []
      cacheRef.current.set(nextScope, items)
      setVariables(items)
    } catch (reason) {
      showError(reason)
    } finally {
      if (!silent) setLoading(false)
    }
  }, [])

  useEffect(() => {
    if (!active) return
    const cached = cacheRef.current.get(scope)
    if (cached) {
      setVariables(cached)
      setLoading(false)
      void load(scope, true)
    } else {
      void load(scope)
    }
  }, [active, load, scope])

  const visibleVariables = useMemo(() => {
    const keyword = query.trim().toLowerCase()
    const filtered = keyword
      ? variables.filter((variable) => `${variable.name}\n${variable.value}`.toLowerCase().includes(keyword))
      : variables
    return filtered.toSorted((left, right) => {
      const leftPath = left.name.toLowerCase() === "path"
      const rightPath = right.name.toLowerCase() === "path"
      if (leftPath !== rightPath) return leftPath ? -1 : 1
      return left.name.localeCompare(right.name, "zh-CN", { sensitivity: "base" })
    })
  }, [query, variables])

  const openCreate = () => {
    setEditorVariable({ name: "", value: "", scope, expandable: false })
    setPreviousName("")
    setEditorOpen(true)
  }

  const openEdit = (variable: EnvironmentVariable) => {
    if (variable.name.toLowerCase() === "path") {
      setPathOpen(true)
      return
    }
    setEditorVariable({ ...variable })
    setPreviousName(variable.name)
    setEditorOpen(true)
  }

  const saveVariable = async () => {
    if (!editorVariable?.name.trim()) return
    setSaving(true)
    try {
      await EnvironmentVariableService.Save({ previousName, variable: editorVariable })
      setEditorOpen(false)
      await load(scope, true)
    } catch (reason) {
      showError(reason, "无法保存环境变量")
    } finally {
      setSaving(false)
    }
  }

  const confirmDelete = async () => {
    if (!deleteVariable) return
    setSaving(true)
    try {
      await EnvironmentVariableService.Delete(deleteVariable.scope, deleteVariable.name)
      setDeleteOpen(false)
      await load(scope, true)
    } catch (reason) {
      showError(reason)
    } finally {
      setSaving(false)
    }
  }

  return (
    <div className="flex h-full min-h-0 w-full flex-col p-4 lg:p-6">
      <div className="flex shrink-0 flex-col gap-3">
        <div className="flex items-center justify-between gap-3">
          <div className="flex min-w-0 items-center gap-2">
            <SidebarTrigger className="-ml-1 shrink-0" />
            <h1 className="min-w-0 text-xl font-semibold tracking-tight">环境变量</h1>
          </div>
          <div className="flex shrink-0 items-center gap-2 whitespace-nowrap">
            <Button variant="outline" onClick={() => void load(scope, variables.length > 0)} disabled={loading}>
              <RefreshCwIcon data-icon="inline-start" />
              刷新
            </Button>
            <Button onClick={openCreate}>
              <PlusIcon data-icon="inline-start" />
              添加变量
            </Button>
          </div>
        </div>

        <div className="flex flex-wrap items-center gap-2 rounded-lg border bg-card p-2">
          <Tabs className="min-w-[12rem] flex-[0_1_18rem]" value={scope} onValueChange={(value) => setScope(value as Scope)}>
            <TabsList className="grid w-full grid-cols-2">
              <TabsTrigger value="system">系统</TabsTrigger>
              <TabsTrigger value="user">用户</TabsTrigger>
            </TabsList>
          </Tabs>
          <InputGroup className="w-full min-w-0 flex-[1_1_18rem]">
            <InputGroupAddon><SearchIcon /></InputGroupAddon>
            <InputGroupInput value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索变量" />
          </InputGroup>
        </div>
      </div>

      <ScrollArea className="-mr-4 min-h-0 flex-1 pr-4 lg:-mr-6 lg:pr-6">
        <div className="flex min-h-full flex-col pt-3 pb-1">
          {loading ? (
            <div className="flex min-h-40 flex-1 items-center justify-center" aria-label="加载中">
              <LoaderCircleIcon className="size-5 animate-spin text-muted-foreground" />
            </div>
          ) : visibleVariables.length ? (
            <div className="overflow-hidden rounded-lg border bg-card">
              {visibleVariables.map((variable) => (
                <ContextMenu key={variable.name.toLowerCase()}>
                  <ContextMenuTrigger className="flex h-10 min-w-0 items-stretch border-b last:border-b-0 hover:bg-muted/30">
                    <button
                      type="button"
                      className="grid min-w-0 flex-1 cursor-pointer grid-cols-[minmax(8rem,14rem)_minmax(0,1fr)] items-center gap-3 px-3 text-left"
                      onClick={() => openEdit(variable)}
                    >
                      <TruncatedText value={variable.name} className="text-sm font-medium" />
                      <TruncatedText value={variable.value} className="font-mono text-xs text-muted-foreground" />
                    </button>
                  </ContextMenuTrigger>
                  <ContextMenuContent className="w-28">
                    <ContextMenuItem onClick={() => {
                      setDeleteVariable(variable)
                      setDeleteOpen(true)
                    }}>
                      <Trash2Icon />
                      删除
                    </ContextMenuItem>
                  </ContextMenuContent>
                </ContextMenu>
              ))}
            </div>
          ) : (
            <Empty className="min-h-48 border">
              <EmptyHeader><EmptyTitle>{query ? "没有匹配的变量" : "没有环境变量"}</EmptyTitle></EmptyHeader>
            </Empty>
          )}
        </div>
      </ScrollArea>

      <Dialog
        open={editorOpen}
        onOpenChange={setEditorOpen}
        onOpenChangeComplete={(next) => {
          if (!next) {
          setEditorVariable(null)
          setPreviousName("")
          }
        }}
      >
        <DialogContent className="!w-[min(92vw,44rem)] !max-w-none">
          <DialogHeader>
            <DialogTitle>{previousName ? "编辑环境变量" : "添加环境变量"}</DialogTitle>
          </DialogHeader>
          {editorVariable ? (
            <FieldGroup>
              <Field>
                <FieldLabel htmlFor="env-name">名称</FieldLabel>
                <Input id="env-name" value={editorVariable.name} onChange={(event) => setEditorVariable({ ...editorVariable, name: event.target.value })} autoFocus={!previousName} />
              </Field>
              <Field>
                <FieldLabel htmlFor="env-value">值</FieldLabel>
                <Textarea
                  id="env-value"
                  value={editorVariable.value}
                  onChange={(event) => setEditorVariable({ ...editorVariable, value: event.target.value })}
                  className="min-h-48 resize-y font-mono text-xs"
                />
              </Field>
            </FieldGroup>
          ) : null}
          <DialogFooter>
            <Button variant="outline" onClick={() => setEditorOpen(false)} disabled={saving}>取消</Button>
            <Button onClick={() => void saveVariable()} disabled={saving || !editorVariable?.name.trim()}>保存</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>

      <PathEditorDialog open={pathOpen} scope={scope} onOpenChange={setPathOpen} onSaved={() => load(scope, true)} />

      <Dialog
        open={deleteOpen}
        onOpenChange={setDeleteOpen}
        onOpenChangeComplete={(next) => !next && setDeleteVariable(null)}
      >
        <DialogContent>
          <DialogHeader>
            <DialogTitle>删除环境变量</DialogTitle>
            <DialogDescription>删除“{deleteVariable?.name}”？</DialogDescription>
          </DialogHeader>
          <DialogFooter>
            <Button variant="outline" onClick={() => setDeleteOpen(false)} disabled={saving}>取消</Button>
            <Button onClick={() => void confirmDelete()} disabled={saving}>删除</Button>
          </DialogFooter>
        </DialogContent>
      </Dialog>
    </div>
  )
}
