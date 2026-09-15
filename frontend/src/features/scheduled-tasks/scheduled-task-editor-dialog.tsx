import { useEffect, useState, type ReactNode } from "react"
import {
  CalendarClockIcon,
  FileUpIcon,
  FolderOpenIcon,
  LoaderCircleIcon,
  SaveIcon,
  ShieldCheckIcon,
  XIcon,
} from "lucide-react"

import {
  ScheduledTaskService,
  type ScheduledTaskEdit,
} from "../../../bindings/win-admin"
import { Button } from "@/components/ui/button"
import {
  Dialog,
  DialogContent,
  DialogFooter,
  DialogHeader,
  DialogTitle,
} from "@/components/ui/dialog"
import { Field, FieldGroup, FieldLabel } from "@/components/ui/field"
import { Input } from "@/components/ui/input"
import {
  InputGroup,
  InputGroupAddon,
  InputGroupButton,
  InputGroupInput,
} from "@/components/ui/input-group"
import { ScrollArea } from "@/components/ui/scroll-area"
import {
  Select,
  SelectContent,
  SelectItem,
  SelectTrigger,
  SelectValue,
} from "@/components/ui/select"
import { Switch } from "@/components/ui/switch"
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/components/ui/tabs"
import { Textarea } from "@/components/ui/textarea"
import { ToggleGroup, ToggleGroupItem } from "@/components/ui/toggle-group"
import { showError, showErrorMessage } from "@/lib/error-toast"
import {
  cloneScheduledTask,
  defaultStartBoundary,
  formatTaskDuration,
  parseTaskDurationSeconds,
  TASK_COMPATIBILITY_OPTIONS,
  TRIGGERS,
  type DurationUnit,
  type TriggerType,
  validateScheduledTask,
  WEEKDAYS,
} from "./scheduled-task-model"

type ScheduledTaskEditorDialogProps = {
  open: boolean
  task: ScheduledTaskEdit | null
  previousPath: string
  title?: string
  allowXMLImport?: boolean
  templateName?: string
  onTemplateNameChange?: (name: string) => void
  onOpenChange: (open: boolean) => void
  onOpenChangeComplete?: (open: boolean) => void
  onSave: (task: ScheduledTaskEdit, previousPath: string, password: string) => Promise<void>
}

const INSTANCE_POLICIES = [
  { value: "ignoreNew", label: "忽略新实例" },
  { value: "parallel", label: "并行运行" },
  { value: "queue", label: "排队运行" },
  { value: "stopExisting", label: "停止现有实例" },
]

const LOGON_MODES = [
  { value: "3", label: "只在用户登录时运行" },
  { value: "1", label: "不管用户是否登录都要运行" },
]

const DURATION_UNITS: Array<{ value: DurationUnit; label: string }> = [
  { value: "seconds", label: "秒" },
  { value: "minutes", label: "分钟" },
  { value: "hours", label: "小时" },
  { value: "days", label: "天" },
]

export function ScheduledTaskEditorDialog({
  open,
  task,
  previousPath,
  title,
  allowXMLImport = false,
  templateName,
  onTemplateNameChange,
  onOpenChange,
  onOpenChangeComplete,
  onSave,
}: ScheduledTaskEditorDialogProps) {
  const [draft, setDraft] = useState<ScheduledTaskEdit | null>(task ? cloneScheduledTask(task) : null)
  const [saving, setSaving] = useState(false)
  const [picking, setPicking] = useState(false)
  const [importingXML, setImportingXML] = useState(false)
  const [password, setPassword] = useState("")
  const [restartCountPreset, setRestartCountPreset] = useState(task?.settings.restartCount || 3)

  useEffect(() => {
    setDraft(task ? cloneScheduledTask(task) : null)
    setPassword("")
    setRestartCountPreset(task?.settings.restartCount || 3)
  }, [task, open])

  if (!draft) return null

  const templateMode = templateName !== undefined

  const submit = async () => {
    if (templateMode && !templateName.trim()) {
      showErrorMessage("模板名称不能为空", "无法保存")
      return
    }
    const taskToSave = templateMode ? { ...draft, name: templateName.trim() } : draft
    const validationError = validateScheduledTask(taskToSave)
    if (validationError) {
      showErrorMessage(validationError, "无法保存")
      return
    }
    if (!templateMode && taskToSave.logonType === 1 && !password) {
      showErrorMessage("不管用户是否登录都要运行时需要输入账户密码", "无法保存")
      return
    }
    setSaving(true)
    try {
      await onSave(taskToSave, previousPath, templateMode ? "" : password)
      onOpenChange(false)
    } catch (reason) {
      showError(reason, "无法保存")
    } finally {
      setSaving(false)
    }
  }

  const chooseExecutable = async () => {
    setPicking(true)
    try {
      const command = await ScheduledTaskService.PickExecutable()
      if (command) {
        setDraft((current) => current ? {
          ...current,
          action: { ...current.action, command },
        } : current)
      }
    } catch (reason) {
      showError(reason)
    } finally {
      setPicking(false)
    }
  }

  const resolveExecutable = async () => {
    const command = draft.action.command.trim()
    if (!command) return
    try {
      const resolved = await ScheduledTaskService.ResolveExecutablePath(command)
      if (resolved && resolved !== draft.action.command) {
        setDraft({
          ...draft,
          action: { ...draft.action, command: resolved },
        })
      }
    } catch (reason) {
      showError(reason, "无法解析程序路径")
    }
  }

  const importXML = async () => {
    setImportingXML(true)
    try {
      const imported = await ScheduledTaskService.ImportXML()
      if (imported) {
        setDraft(cloneScheduledTask(imported))
        setPassword("")
        setRestartCountPreset(imported.settings.restartCount || 3)
      }
    } catch (reason) {
      showError(reason, "无法导入 XML")
    } finally {
      setImportingXML(false)
    }
  }

  const updateSetting = <K extends keyof ScheduledTaskEdit["settings"]>(
    key: K,
    value: ScheduledTaskEdit["settings"][K],
  ) => {
    setDraft({
      ...draft,
      settings: { ...draft.settings, [key]: value },
    })
  }

  const triggerType = draft.trigger.type as TriggerType
  const hasStartBoundary = triggerType === "once" || triggerType === "daily" || triggerType === "weekly"

  return (
    <Dialog open={open} onOpenChange={onOpenChange} onOpenChangeComplete={onOpenChangeComplete}>
      <DialogContent className="flex h-[min(680px,calc(100vh-2rem))] !w-[min(92vw,56rem)] min-w-0 !max-w-none flex-col overflow-hidden">
        <DialogHeader>
          <div className="flex min-w-0 items-center justify-between gap-3 pr-8">
            <DialogTitle className="min-w-0 truncate">{title ?? (previousPath ? "编辑计划任务" : "添加计划任务")}</DialogTitle>
            {allowXMLImport ? (
              <Button variant="outline" size="sm" className="shrink-0" onClick={() => void importXML()} disabled={importingXML || saving}>
                {importingXML ? <LoaderCircleIcon data-icon="inline-start" className="size-4 self-center animate-spin" /> : <FileUpIcon data-icon="inline-start" />}
                导入 XML
              </Button>
            ) : null}
          </div>
        </DialogHeader>

        <Tabs defaultValue="general" className="flex min-h-0 flex-1 flex-col">
          <TabsList className="grid w-full shrink-0 grid-cols-5">
            <TabsTrigger value="general">常规</TabsTrigger>
            <TabsTrigger value="trigger">触发器</TabsTrigger>
            <TabsTrigger value="action">操作</TabsTrigger>
            <TabsTrigger value="conditions">条件</TabsTrigger>
            <TabsTrigger value="settings">设置</TabsTrigger>
          </TabsList>

          <ScrollArea className="min-h-0 min-w-0 flex-1 pr-3">
            <TabsContent value="general" className="m-0 px-1 py-2">
              <FieldGroup className="min-w-0 gap-4">
                <FieldGroup className="grid min-w-0 grid-cols-1 gap-4 md:grid-cols-2">
                  <Field className="min-w-0">
                    <FieldLabel htmlFor={templateMode ? "template-editor-name" : "task-name"}>
                      {templateMode ? "模板名称" : "任务名称"}
                    </FieldLabel>
                    <Input
                      id={templateMode ? "template-editor-name" : "task-name"}
                      value={templateMode ? templateName : draft.name}
                      onChange={(event) => {
                        if (templateMode) onTemplateNameChange?.(event.target.value)
                        else setDraft({ ...draft, name: event.target.value })
                      }}
                      autoFocus={!previousPath}
                    />
                  </Field>
                  <Field className="min-w-0">
                    <FieldLabel htmlFor="task-folder">目录</FieldLabel>
                    <Input
                      id="task-folder"
                      value={draft.folder}
                      onChange={(event) => setDraft({ ...draft, folder: event.target.value })}
                    />
                  </Field>
                </FieldGroup>

                <Field className="min-w-0">
                  <FieldLabel htmlFor="task-user">运行账户</FieldLabel>
                  <Input
                    id="task-user"
                    value={draft.userId}
                    onChange={(event) => setDraft({ ...draft, userId: event.target.value })}
                    placeholder="留空使用当前用户"
                  />
                </Field>

                <Field className="min-w-0">
                  <FieldLabel>登录方式</FieldLabel>
                  <Select
                    items={logonModeOptions(draft.logonType)}
                    value={draft.logonType === 2 ? "1" : String(draft.logonType)}
                    onValueChange={(value) => {
                      if (!value) return
                      const next = Number(value)
                      setDraft({ ...draft, logonType: next })
                      if (next !== 1) setPassword("")
                    }}
                  >
                    <SelectTrigger className="w-full"><SelectValue /></SelectTrigger>
                    <SelectContent>
                      {logonModeOptions(draft.logonType).map((option) => (
                        <SelectItem key={option.value} value={option.value}>{option.label}</SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </Field>

                {draft.logonType === 1 || draft.logonType === 2 ? (
                  <SwitchRow
                    label="不存储密码，仅访问本机资源"
                    separated={false}
                    checked={draft.logonType === 2}
                    onCheckedChange={(checked) => {
                      setDraft({ ...draft, logonType: checked ? 2 : 1 })
                      if (checked) setPassword("")
                    }}
                  />
                ) : null}

                {!templateMode && draft.logonType === 1 ? (
                  <Field className="min-w-0">
                    <FieldLabel htmlFor="task-password">账户密码</FieldLabel>
                    <Input
                      id="task-password"
                      type="password"
                      value={password}
                      autoComplete="new-password"
                      placeholder={previousPath ? "重新保存时需要再次输入密码" : "输入运行账户密码"}
                      onChange={(event) => setPassword(event.target.value)}
                    />
                  </Field>
                ) : null}

                <FieldGroup className="grid min-w-0 grid-cols-1 gap-3 md:grid-cols-2">
                  <SwitchRow
                    label="启用"
                    separated={false}
                    checked={draft.enabled}
                    onCheckedChange={(enabled) => setDraft({ ...draft, enabled })}
                  />
                  <SwitchRow
                    label="最高权限"
                    separated={false}
                    icon={<ShieldCheckIcon className="size-4" />}
                    checked={draft.runLevel === "highest"}
                    onCheckedChange={(highest) => setDraft({
                      ...draft,
                      runLevel: highest ? "highest" : "least",
                    })}
                  />
                </FieldGroup>

                <Field>
                  <FieldLabel htmlFor="task-description">描述</FieldLabel>
                  <Textarea
                    id="task-description"
                    value={draft.description}
                    onChange={(event) => setDraft({ ...draft, description: event.target.value })}
                    className="min-h-24 resize-y"
                  />
                </Field>

                <FieldGroup className="grid min-w-0 grid-cols-1 gap-4 md:grid-cols-[minmax(0,1fr)_minmax(18rem,1fr)] md:items-end">
                  <div className="overflow-hidden rounded-lg border">
                    <SwitchRow
                      label="隐藏"
                      checked={draft.settings.hidden}
                      onCheckedChange={(checked) => updateSetting("hidden", checked)}
                    />
                  </div>
                  <Field>
                    <FieldLabel>配置</FieldLabel>
                    <Select
                      items={compatibilityOptions(draft.settings.compatibility)}
                      value={String(draft.settings.compatibility)}
                      onValueChange={(value) => value && updateSetting("compatibility", Number(value))}
                    >
                      <SelectTrigger className="w-full"><SelectValue /></SelectTrigger>
                      <SelectContent alignItemWithTrigger={false}>
                        {compatibilityOptions(draft.settings.compatibility).map((option) => (
                          <SelectItem key={option.value} value={option.value}>{option.label}</SelectItem>
                        ))}
                      </SelectContent>
                    </Select>
                  </Field>
                </FieldGroup>
              </FieldGroup>
            </TabsContent>

            <TabsContent value="trigger" className="m-0 px-1 py-2">
              <FieldGroup className="min-w-0 gap-4">
                <Field className="min-w-0">
                  <FieldLabel>触发器</FieldLabel>
                  <Select
                    items={TRIGGERS}
                    value={triggerType}
                    onValueChange={(value) => {
                      if (!value) return
                      const type = value as TriggerType
                      const needsBoundary = type === "once" || type === "daily" || type === "weekly"
                      setDraft({
                        ...draft,
                        trigger: {
                          ...draft.trigger,
                          type,
                          startBoundary: needsBoundary && !draft.trigger.startBoundary
                            ? defaultStartBoundary()
                            : draft.trigger.startBoundary,
                        },
                      })
                    }}
                  >
                    <SelectTrigger className="w-full">
                      <CalendarClockIcon />
                      <SelectValue />
                    </SelectTrigger>
                    <SelectContent align="start">
                      {TRIGGERS.map((trigger) => (
                        <SelectItem key={trigger.value} value={trigger.value}>
                          {trigger.label}
                        </SelectItem>
                      ))}
                    </SelectContent>
                  </Select>
                </Field>

                {hasStartBoundary ? (
                  <FieldGroup className="grid min-w-0 grid-cols-1 gap-4 md:grid-cols-2">
                    <Field className="min-w-0">
                      <FieldLabel htmlFor="task-start-boundary">时间</FieldLabel>
                      <Input
                        id="task-start-boundary"
                        type="datetime-local"
                        value={toDateTimeLocalValue(draft.trigger.startBoundary)}
                        onChange={(event) => setDraft({
                          ...draft,
                          trigger: { ...draft.trigger, startBoundary: event.target.value },
                        })}
                      />
                    </Field>
                    {triggerType === "daily" || triggerType === "weekly" ? (
                      <Field className="min-w-0">
                        <FieldLabel htmlFor="task-trigger-interval">
                          {triggerType === "daily" ? "间隔（天）" : "间隔（周）"}
                        </FieldLabel>
                        <Input
                          id="task-trigger-interval"
                          type="number"
                          min={1}
                          max={triggerType === "daily" ? 365 : 52}
                          value={draft.trigger.interval || 1}
                          onChange={(event) => setDraft({
                            ...draft,
                            trigger: {
                              ...draft.trigger,
                              interval: Number(event.target.value) || 1,
                            },
                          })}
                        />
                      </Field>
                    ) : null}
                  </FieldGroup>
                ) : null}

                {triggerType === "weekly" ? (
                  <Field>
                    <FieldLabel>星期</FieldLabel>
                    <ToggleGroup
                      value={draft.trigger.daysOfWeek ?? []}
                      onValueChange={(days) => setDraft({
                        ...draft,
                        trigger: { ...draft.trigger, daysOfWeek: days },
                      })}
                      variant="outline"
                      spacing={0}
                    >
                      {WEEKDAYS.map((day) => (
                        <ToggleGroupItem key={day.value} value={day.value} aria-label={`星期${day.label}`}>
                          {day.label}
                        </ToggleGroupItem>
                      ))}
                    </ToggleGroup>
                  </Field>
                ) : null}

                {triggerType === "logon" ? (
                  <Field>
                    <FieldLabel htmlFor="task-logon-user">登录用户</FieldLabel>
                    <Input
                      id="task-logon-user"
                      value={draft.trigger.userId}
                      onChange={(event) => setDraft({
                        ...draft,
                        trigger: { ...draft.trigger, userId: event.target.value },
                      })}
                      placeholder="留空匹配任意用户"
                    />
                  </Field>
                ) : null}
              </FieldGroup>
            </TabsContent>

            <TabsContent value="action" className="m-0 px-1 py-2">
              <FieldGroup className="min-w-0 gap-4">
                <Field className="min-w-0">
                  <FieldLabel htmlFor="task-command">程序</FieldLabel>
                  <InputGroup>
                    <InputGroupAddon>
                      <FolderOpenIcon />
                    </InputGroupAddon>
                    <InputGroupInput
                      id="task-command"
                      value={draft.action.command}
                      onChange={(event) => setDraft({
                        ...draft,
                        action: { ...draft.action, command: event.target.value },
                      })}
                      onBlur={() => void resolveExecutable()}
                    />
                    <InputGroupAddon align="inline-end">
                      <InputGroupButton onClick={() => void chooseExecutable()} disabled={picking}>
                        选择
                      </InputGroupButton>
                    </InputGroupAddon>
                  </InputGroup>
                </Field>

                <Field>
                  <FieldLabel htmlFor="task-arguments">参数</FieldLabel>
                  <Textarea
                    id="task-arguments"
                    value={draft.action.arguments}
                    onChange={(event) => setDraft({
                      ...draft,
                      action: { ...draft.action, arguments: event.target.value },
                    })}
                    className="min-h-24 font-mono text-xs"
                  />
                </Field>

                <Field>
                  <FieldLabel htmlFor="task-working-directory">工作目录</FieldLabel>
                  <Input
                    id="task-working-directory"
                    value={draft.action.workingDirectory}
                    onChange={(event) => setDraft({
                      ...draft,
                      action: { ...draft.action, workingDirectory: event.target.value },
                    })}
                  />
                </Field>
              </FieldGroup>
            </TabsContent>

            <TabsContent value="conditions" className="m-0 px-1 py-2">
              <FieldGroup className="min-w-0 gap-4">
                <div className="overflow-hidden rounded-lg border">
                  <SwitchRow label="使用电池时不启动" checked={draft.settings.disallowStartIfOnBatteries} onCheckedChange={(checked) => updateSetting("disallowStartIfOnBatteries", checked)} />
                  <SwitchRow label="切换到电池时停止" checked={draft.settings.stopIfGoingOnBatteries} onCheckedChange={(checked) => updateSetting("stopIfGoingOnBatteries", checked)} />
                  <SwitchRow label="仅网络可用时运行" checked={draft.settings.runOnlyIfNetworkAvailable} onCheckedChange={(checked) => updateSetting("runOnlyIfNetworkAvailable", checked)} />
                  <SwitchRow label="唤醒计算机运行" checked={draft.settings.wakeToRun} onCheckedChange={(checked) => updateSetting("wakeToRun", checked)} />
                </div>

                <div className="overflow-hidden rounded-lg border">
                  <SwitchRow label="仅计算机空闲时运行" checked={draft.settings.runOnlyIfIdle} onCheckedChange={(checked) => updateSetting("runOnlyIfIdle", checked)} />
                  <FieldGroup className="grid min-w-0 grid-cols-1 gap-4 border-b p-3 md:grid-cols-2">
                    <Field>
                      <FieldLabel htmlFor="task-idle-duration">空闲持续时间</FieldLabel>
                      <DurationInput
                        id="task-idle-duration"
                        value={draft.settings.idleDuration}
                        zeroUnit="minutes"
                        minimumSeconds={60}
                        disabled={!draft.settings.runOnlyIfIdle}
                        onChange={(value) => updateSetting("idleDuration", value)}
                      />
                    </Field>
                    <Field>
                      <FieldLabel htmlFor="task-idle-wait-timeout">等待空闲时间</FieldLabel>
                      <DurationInput
                        id="task-idle-wait-timeout"
                        value={draft.settings.idleWaitTimeout}
                        zeroUnit="hours"
                        minimumSeconds={60}
                        disabled={!draft.settings.runOnlyIfIdle}
                        onChange={(value) => updateSetting("idleWaitTimeout", value)}
                      />
                    </Field>
                  </FieldGroup>
                  <SwitchRow label="空闲结束时停止" checked={draft.settings.stopOnIdleEnd} disabled={!draft.settings.runOnlyIfIdle} onCheckedChange={(checked) => updateSetting("stopOnIdleEnd", checked)} />
                  <SwitchRow label="再次空闲时重新启动" checked={draft.settings.restartOnIdle} disabled={!draft.settings.runOnlyIfIdle} onCheckedChange={(checked) => updateSetting("restartOnIdle", checked)} />
                </div>
              </FieldGroup>
            </TabsContent>

            <TabsContent value="settings" className="m-0 px-1 py-2">
              <FieldGroup className="min-w-0 gap-4">
                <div className="overflow-hidden rounded-lg border">
                  <SwitchRow label="允许按需运行" checked={draft.settings.allowStartOnDemand} onCheckedChange={(checked) => updateSetting("allowStartOnDemand", checked)} />
                  <SwitchRow label="错过计划后尽快运行" checked={draft.settings.startWhenAvailable} onCheckedChange={(checked) => updateSetting("startWhenAvailable", checked)} />
                  <SwitchRow label="允许强制终止" checked={draft.settings.allowHardTerminate} onCheckedChange={(checked) => updateSetting("allowHardTerminate", checked)} />
                </div>

                <Field>
                  <FieldLabel>任务已在运行时</FieldLabel>
                  <Select items={INSTANCE_POLICIES} value={draft.settings.multipleInstancesPolicy} onValueChange={(value) => value && updateSetting("multipleInstancesPolicy", value)}>
                    <SelectTrigger className="w-full"><SelectValue /></SelectTrigger>
                    <SelectContent>
                      {INSTANCE_POLICIES.map((policy) => <SelectItem key={policy.value} value={policy.value}>{policy.label}</SelectItem>)}
                    </SelectContent>
                  </Select>
                </Field>

                <FieldGroup className="grid min-w-0 grid-cols-1 gap-4 md:grid-cols-2">
                  <Field>
                    <FieldLabel htmlFor="task-time-limit">执行时间限制</FieldLabel>
                    <DurationInput
                      id="task-time-limit"
                      value={draft.settings.executionTimeLimit}
                      zeroUnit="days"
                      unlimited
                      onChange={(value) => updateSetting("executionTimeLimit", value)}
                    />
                  </Field>
                  <Field>
                    <div className="flex min-w-0 items-center justify-between gap-3">
                      <FieldLabel htmlFor="task-delete-expired-after">无后续计划后删除</FieldLabel>
                      <Switch
                        size="sm"
                        aria-label="无后续计划后删除"
                        checked={Boolean(draft.settings.deleteExpiredTaskAfter)}
                        onCheckedChange={(checked) => updateSetting("deleteExpiredTaskAfter", checked ? "P30D" : "")}
                      />
                    </div>
                    <DurationInput
                      id="task-delete-expired-after"
                      value={draft.settings.deleteExpiredTaskAfter || "P30D"}
                      zeroUnit="days"
                      disabled={!draft.settings.deleteExpiredTaskAfter}
                      onChange={(value) => updateSetting("deleteExpiredTaskAfter", value)}
                    />
                  </Field>
                </FieldGroup>

                <Field>
                  <div className="flex min-w-0 items-center justify-between gap-3">
                    <FieldLabel>失败时重新启动</FieldLabel>
                    <Switch
                      size="sm"
                      aria-label="失败时重新启动"
                      checked={draft.settings.restartCount > 0}
                      onCheckedChange={(checked) => {
                        if (checked) {
                          updateSetting("restartCount", restartCountPreset || 3)
                        } else {
                          if (draft.settings.restartCount > 0) setRestartCountPreset(draft.settings.restartCount)
                          updateSetting("restartCount", 0)
                        }
                      }}
                    />
                  </div>
                  <FieldGroup className="grid min-w-0 grid-cols-1 gap-4 md:grid-cols-2">
                    <Field>
                      <FieldLabel htmlFor="task-restart-interval">失败重启间隔</FieldLabel>
                      <DurationInput
                        id="task-restart-interval"
                        value={draft.settings.restartInterval}
                        zeroUnit="minutes"
                        disabled={draft.settings.restartCount === 0}
                        onChange={(value) => updateSetting("restartInterval", value)}
                      />
                    </Field>
                    <Field>
                      <FieldLabel htmlFor="task-restart-count">失败重启次数</FieldLabel>
                      <Input
                        id="task-restart-count"
                        type="number"
                        min={1}
                        max={255}
                        disabled={draft.settings.restartCount === 0}
                        value={draft.settings.restartCount || restartCountPreset}
                        onChange={(event) => {
                          const value = Number(event.target.value)
                          setRestartCountPreset(value)
                          updateSetting("restartCount", value)
                        }}
                      />
                    </Field>
                  </FieldGroup>
                </Field>
              </FieldGroup>
            </TabsContent>
          </ScrollArea>
        </Tabs>

        <DialogFooter>
          <Button variant="outline" onClick={() => onOpenChange(false)} disabled={saving}>
            <XIcon data-icon="inline-start" />取消
          </Button>
          <Button onClick={() => void submit()} disabled={saving}>
            <SaveIcon data-icon="inline-start" />保存
          </Button>
        </DialogFooter>
      </DialogContent>
    </Dialog>
  )
}

type SwitchRowProps = {
  label: string
  checked: boolean
  icon?: ReactNode
  separated?: boolean
  disabled?: boolean
  onCheckedChange: (checked: boolean) => void
}

function SwitchRow({ label, checked, icon, separated = true, disabled = false, onCheckedChange }: SwitchRowProps) {
  return (
    <label className={`flex min-h-9 min-w-0 items-center justify-between gap-3 px-3 text-sm transition-colors ${disabled ? "cursor-not-allowed opacity-50" : "cursor-pointer hover:bg-muted/30"} ${separated ? "border-b last:border-b-0" : "rounded-lg border"}`}>
      <span className="flex min-w-0 items-center gap-2">{icon}<span>{label}</span></span>
      <Switch size="sm" checked={checked} disabled={disabled} onCheckedChange={onCheckedChange} />
    </label>
  )
}

type DurationInputProps = {
  id: string
  value: string
  zeroUnit: DurationUnit
  unlimited?: boolean
  disabled?: boolean
  minimumSeconds?: number
  onChange: (value: string) => void
}

function DurationInput({ id, value, zeroUnit, unlimited = false, disabled = false, minimumSeconds = 0, onChange }: DurationInputProps) {
  const duration = durationInputValue(value, zeroUnit)
  const hasAmount = duration.amount !== ""

  return (
    <div className={hasAmount ? "grid min-w-0 grid-cols-[minmax(0,1fr)_7rem] gap-2" : "min-w-0"}>
      <Input
        id={id}
        type="number"
        min={1}
        step={1}
        value={duration.amount}
        disabled={disabled}
        placeholder={unlimited ? "无限制" : undefined}
        onChange={(event) => {
          if (!event.target.value) {
            onChange("PT0S")
            return
          }
          const amount = Number(event.target.value)
          if (Number.isFinite(amount) && amount >= 0) {
            const next = formatTaskDuration(amount, duration.unit)
            const seconds = parseTaskDurationSeconds(next)
            if (seconds !== null && seconds >= minimumSeconds) onChange(next)
          }
        }}
      />
      {hasAmount ? (
        <Select
          items={DURATION_UNITS}
          value={duration.unit}
          disabled={disabled}
          onValueChange={(unit) => unit && onChange(formatTaskDuration(Number(duration.amount), unit as DurationUnit))}
        >
          <SelectTrigger className="w-full"><SelectValue /></SelectTrigger>
          <SelectContent>
            {DURATION_UNITS.map((unit) => <SelectItem key={unit.value} value={unit.value}>{unit.label}</SelectItem>)}
          </SelectContent>
        </Select>
      ) : null}
    </div>
  )
}

function durationInputValue(value: string, zeroUnit: DurationUnit): { amount: string; unit: DurationUnit } {
  const seconds = parseTaskDurationSeconds(value)
  if (!seconds) return { amount: "", unit: zeroUnit }

  const candidates: Array<[DurationUnit, number]> = [
    ["days", 24 * 60 * 60],
    ["hours", 60 * 60],
    ["minutes", 60],
    ["seconds", 1],
  ]
  for (const [unit, unitSeconds] of candidates) {
    if (seconds % unitSeconds === 0) return { amount: String(seconds / unitSeconds), unit }
  }
  return { amount: String(Math.round(seconds)), unit: "seconds" }
}

function compatibilityOptions(current: number): Array<{ value: string; label: string }> {
  const options = [...TASK_COMPATIBILITY_OPTIONS]
  const currentValue = String(current)
  if (!options.some((option) => option.value === currentValue)) {
    options.push({ value: currentValue, label: `兼容级别 ${current}` })
  }
  return options
}

function logonModeOptions(current: number): Array<{ value: string; label: string }> {
  const options = [...LOGON_MODES]
  if (current === 5) options.push({ value: "5", label: "服务账户" })
  return options
}

function toDateTimeLocalValue(value: string): string {
  if (!value) return ""
  return value.length >= 16 ? value.slice(0, 16) : value
}

