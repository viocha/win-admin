import { useCallback, useEffect, useMemo, useRef, useState } from "react"
import {
  AppWindowIcon,
  ArrowDownIcon,
  ArrowUpDownIcon,
  ArrowUpIcon,
  CopyIcon,
  FolderOpenIcon,
  ListTreeIcon,
  LoaderCircleIcon,
  RefreshCwIcon,
  SearchIcon,
  Trash2Icon,
} from "lucide-react"

import {
  AppUninstallService,
  type InstalledApp,
} from "../../../bindings/win-admin"
import { Button } from "@/components/ui/button"
import {
  ContextMenu,
  ContextMenuContent,
  ContextMenuItem,
  ContextMenuSeparator,
  ContextMenuTrigger,
} from "@/components/ui/context-menu"
import { Empty, EmptyHeader, EmptyTitle } from "@/components/ui/empty"
import {
  InputGroup,
  InputGroupAddon,
  InputGroupInput,
} from "@/components/ui/input-group"
import { ScrollArea } from "@/components/ui/scroll-area"
import { SidebarTrigger } from "@/components/ui/sidebar"
import { TruncatedText } from "@/components/ui/truncated-text"
import { showError } from "@/lib/error-toast"
import {
  Table,
  TableBody,
  TableCell,
  TableHead,
  TableHeader,
  TableRow,
} from "@/components/ui/table"

type SortKey = "name" | "size" | "date"
type SortDirection = "asc" | "desc"
type SortState = { key: SortKey; direction: SortDirection }

const SORT_STORAGE_KEY = "win-admin.app-uninstall.sort"

function loadSortState(): SortState {
  try {
    const value = JSON.parse(window.localStorage.getItem(SORT_STORAGE_KEY) || "null") as Partial<SortState> | null
    if (
      value &&
      (value.key === "name" || value.key === "size" || value.key === "date") &&
      (value.direction === "asc" || value.direction === "desc")
    ) {
      return { key: value.key, direction: value.direction }
    }
  } catch {
    // Ignore malformed legacy values.
  }
  return { key: "name", direction: "asc" }
}

export function AppUninstallPage({ active }: { active: boolean }) {
  const [apps, setApps] = useState<InstalledApp[]>([])
  const [loading, setLoading] = useState(true)
  const [query, setQuery] = useState("")
  const [uninstallingId, setUninstallingId] = useState("")
  const [sort, setSort] = useState<SortState>(loadSortState)
  const hasLoadedRef = useRef(false)
  const loadGenerationRef = useRef(0)

  const load = useCallback(async (silent = false) => {
    const generation = ++loadGenerationRef.current
    if (!silent) setLoading(true)
    try {
      const nextApps = (await AppUninstallService.List()) ?? []
      if (generation !== loadGenerationRef.current) return
      setApps(nextApps)
      hasLoadedRef.current = true

      const missingSizeIds = nextApps
        .filter((app) => !app.estimatedSizeKB && app.installLocation)
        .map((app) => app.id)
      if (missingSizeIds.length) {
        void AppUninstallService.Sizes(missingSizeIds)
          .then((sizes) => {
            if (generation !== loadGenerationRef.current || !sizes?.length) return
            const sizeById = new Map(sizes.map((item) => [item.id, item.estimatedSizeKB]))
            setApps((current) => current.map((app) => {
              const estimatedSizeKB = sizeById.get(app.id)
              return estimatedSizeKB === undefined ? app : { ...app, estimatedSizeKB }
            }))
          })
          .catch(() => {})
      }
    } catch (reason) {
      if (generation === loadGenerationRef.current) showError(reason)
    } finally {
      if (!silent && generation === loadGenerationRef.current) setLoading(false)
    }
  }, [])

  useEffect(() => {
    if (!active) return
    void load(hasLoadedRef.current)
  }, [active, load])

  useEffect(() => {
    window.localStorage.setItem(SORT_STORAGE_KEY, JSON.stringify(sort))
  }, [sort])

  const visibleApps = useMemo(() => {
    const keyword = query.trim().toLowerCase()
    const filtered = keyword
      ? apps.filter((app) => `${app.name}\n${app.version}\n${app.publisher}`.toLowerCase().includes(keyword))
      : apps

    return filtered.toSorted((left, right) => compareApps(left, right, sort.key, sort.direction))
  }, [apps, query, sort])

  const changeSort = (nextKey: SortKey) => {
    if (nextKey === sort.key) {
      setSort((current) => ({ ...current, direction: current.direction === "asc" ? "desc" : "asc" }))
      return
    }
    setSort({ key: nextKey, direction: nextKey === "name" ? "asc" : "desc" })
  }

  const uninstall = async (app: InstalledApp) => {
    setUninstallingId(app.id)
    try {
      await AppUninstallService.Uninstall(app.id)
      await load(true)
    } catch (reason) {
      showError(reason)
    } finally {
      setUninstallingId("")
    }
  }

  const openInstallLocation = async (app: InstalledApp) => {
    try {
      await AppUninstallService.OpenInstallLocation(app.id)
    } catch (reason) {
      showError(reason, "无法打开安装位置")
    }
  }

  const openRegistry = async (app: InstalledApp) => {
    try {
      await AppUninstallService.OpenRegistry(app.id)
    } catch (reason) {
      showError(reason, "无法打开注册表位置")
    }
  }

  const copyRegistryPath = async (app: InstalledApp) => {
    try {
      await navigator.clipboard.writeText(app.registryPath)
    } catch (reason) {
      showError(reason, "无法复制注册表路径")
    }
  }

  return (
    <div className="flex h-full min-h-0 w-full flex-col p-4 lg:p-6">
      <div className="flex shrink-0 flex-col gap-3">
        <div className="flex items-center justify-between gap-3">
          <div className="flex min-w-0 items-center gap-2">
            <SidebarTrigger className="-ml-1 shrink-0" />
            <h1 className="min-w-0 text-xl font-semibold tracking-tight">应用卸载</h1>
          </div>
          <Button variant="outline" onClick={() => void load(apps.length > 0)} disabled={loading}>
            <RefreshCwIcon data-icon="inline-start" />
            刷新
          </Button>
        </div>

        <InputGroup className="w-full lg:max-w-sm">
          <InputGroupAddon><SearchIcon /></InputGroupAddon>
          <InputGroupInput value={query} onChange={(event) => setQuery(event.target.value)} placeholder="搜索应用" />
        </InputGroup>
      </div>

      <ScrollArea className="min-h-0 flex-1">
        <div className="flex min-h-full flex-col pt-3 pr-3 pb-1">
          {loading ? (
            <div className="flex min-h-40 flex-1 items-center justify-center" aria-label="加载中">
              <LoaderCircleIcon className="size-5 animate-spin text-muted-foreground" />
            </div>
          ) : visibleApps.length ? (
            <div className="overflow-hidden rounded-lg border bg-card">
              <Table className="table-fixed">
                <TableHeader className="bg-muted/20">
                  <TableRow className="hover:bg-transparent">
                    <TableHead>
                      <SortButton label="名称" active={sort.key === "name"} direction={sort.direction} onClick={() => changeSort("name")} />
                    </TableHead>
                    <TableHead className="hidden w-40 lg:table-cell">发布者</TableHead>
                    <TableHead className="hidden w-28 md:table-cell">版本</TableHead>
                    <TableHead className="w-28">
                      <SortButton label="安装时间" active={sort.key === "date"} direction={sort.direction} onClick={() => changeSort("date")} />
                    </TableHead>
                    <TableHead className="w-24 text-right">
                      <SortButton label="大小" active={sort.key === "size"} direction={sort.direction} onClick={() => changeSort("size")} align="end" />
                    </TableHead>
                    <TableHead className="w-20" />
                  </TableRow>
                </TableHeader>
                <TableBody>
                  {visibleApps.map((app) => (
                    <ContextMenu key={app.id}>
                      <ContextMenuTrigger render={<TableRow />}>
                        <TableCell className="min-w-0">
                          <div className="flex min-w-0 items-center gap-2">
                            <div className="flex size-7 shrink-0 items-center justify-center rounded-md bg-muted/40">
                              {app.icon ? (
                                <img src={app.icon} alt="" className="size-6 object-contain" />
                              ) : (
                                <AppWindowIcon className="size-4 text-muted-foreground" />
                              )}
                            </div>
                            <TruncatedText value={app.name} className="font-medium" />
                          </div>
                        </TableCell>
                        <TableCell className="hidden min-w-0 text-xs text-muted-foreground lg:table-cell">
                          <TruncatedText value={app.publisher || "—"} />
                        </TableCell>
                        <TableCell className="hidden min-w-0 text-xs text-muted-foreground md:table-cell">
                          <TruncatedText value={app.version || "—"} />
                        </TableCell>
                        <TableCell className="text-xs tabular-nums text-muted-foreground">{app.installDate || "—"}</TableCell>
                        <TableCell className="text-right text-xs tabular-nums text-muted-foreground">{formatSize(app.estimatedSizeKB)}</TableCell>
                        <TableCell className="text-right">
                          <Button
                            variant="outline"
                            size="sm"
                            onClick={() => void uninstall(app)}
                            disabled={uninstallingId === app.id}
                          >
                            卸载
                          </Button>
                        </TableCell>
                      </ContextMenuTrigger>
                      <ContextMenuContent className="min-w-max">
                        <ContextMenuItem onClick={() => void openInstallLocation(app)} disabled={!app.installLocation}>
                          <FolderOpenIcon />
                          打开安装位置
                        </ContextMenuItem>
                        <ContextMenuItem onClick={() => void openRegistry(app)}>
                          <ListTreeIcon />
                          打开注册表位置
                        </ContextMenuItem>
                        <ContextMenuItem onClick={() => void copyRegistryPath(app)}>
                          <CopyIcon />
                          复制注册表路径
                        </ContextMenuItem>
                        <ContextMenuSeparator />
                        <ContextMenuItem onClick={() => void uninstall(app)} disabled={uninstallingId === app.id}>
                          <Trash2Icon />
                          卸载
                        </ContextMenuItem>
                      </ContextMenuContent>
                    </ContextMenu>
                  ))}
                </TableBody>
              </Table>
            </div>
          ) : (
            <Empty className="min-h-48 border">
              <EmptyHeader><EmptyTitle>{query ? "没有匹配的应用" : "没有可卸载应用"}</EmptyTitle></EmptyHeader>
            </Empty>
          )}
        </div>
      </ScrollArea>

    </div>
  )
}

type SortButtonProps = {
  label: string
  active: boolean
  direction: SortDirection
  onClick: () => void
  align?: "start" | "end"
}

function SortButton({ label, active, direction, onClick, align = "start" }: SortButtonProps) {
  return (
    <Button variant="ghost" size="xs" className={align === "end" ? "ml-auto" : "-ml-2"} onClick={onClick}>
      {label}
      {active ? (
        direction === "asc" ? <ArrowUpIcon data-icon="inline-end" /> : <ArrowDownIcon data-icon="inline-end" />
      ) : (
        <ArrowUpDownIcon data-icon="inline-end" />
      )}
    </Button>
  )
}

function compareApps(left: InstalledApp, right: InstalledApp, key: SortKey, direction: SortDirection): number {
  if (key === "name") {
    const result = left.name.localeCompare(right.name, "zh-CN", { sensitivity: "base" })
    return direction === "asc" ? result : -result
  }

  if (key === "size") {
    return compareOptionalNumber(left.estimatedSizeKB, right.estimatedSizeKB, direction)
  }

  const dateResult = compareOptionalNumber(installDateValue(left.installDate), installDateValue(right.installDate), direction)
  if (dateResult !== 0) return dateResult

  return compareOptionalNumber(left.installTimestamp, right.installTimestamp, direction)
}

function compareOptionalNumber(left: number, right: number, direction: SortDirection): number {
  const leftMissing = !left
  const rightMissing = !right
  if (leftMissing !== rightMissing) return leftMissing ? 1 : -1
  const result = left - right
  return direction === "asc" ? result : -result
}

function installDateValue(value: string): number {
  const digits = value.replace(/\D/g, "")
  if (digits.length >= 8) return Number(digits.slice(0, 8))
  const parsed = Date.parse(value)
  return Number.isNaN(parsed) ? 0 : parsed
}

function formatSize(sizeKB: number): string {
  if (!sizeKB) return "—"
  const sizeMB = sizeKB / 1024
  if (sizeMB < 1024) return `${sizeMB < 10 ? sizeMB.toFixed(1) : Math.round(sizeMB)} MB`
  const sizeGB = sizeMB / 1024
  return `${sizeGB < 10 ? sizeGB.toFixed(1) : Math.round(sizeGB)} GB`
}
