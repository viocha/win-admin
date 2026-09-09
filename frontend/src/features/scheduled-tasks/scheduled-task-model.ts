import type { ScheduledTaskEdit } from "../../../bindings/win-admin"

export type TriggerType = "manual" | "once" | "daily" | "weekly" | "startup" | "logon"
export type DurationUnit = "seconds" | "minutes" | "hours" | "days"

const DURATION_UNIT_SECONDS: Record<DurationUnit, number> = {
  seconds: 1,
  minutes: 60,
  hours: 60 * 60,
  days: 24 * 60 * 60,
}

export const TRIGGERS: Array<{ value: TriggerType; label: string }> = [
  { value: "manual", label: "手动" },
  { value: "once", label: "一次" },
  { value: "daily", label: "每天" },
  { value: "weekly", label: "每周" },
  { value: "logon", label: "登录时" },
  { value: "startup", label: "启动时" },
]

export const WEEKDAYS = [
  { value: "monday", label: "一" },
  { value: "tuesday", label: "二" },
  { value: "wednesday", label: "三" },
  { value: "thursday", label: "四" },
  { value: "friday", label: "五" },
  { value: "saturday", label: "六" },
  { value: "sunday", label: "日" },
] as const

export function createEmptyScheduledTask(): ScheduledTaskEdit {
  return {
    name: "",
    folder: "\\",
    description: "",
    enabled: true,
    userId: "",
    runLevel: "highest",
    action: {
      command: "",
      arguments: "",
      workingDirectory: "",
    },
    trigger: {
      type: "manual",
      startBoundary: defaultStartBoundary(),
      interval: 1,
      daysOfWeek: [weekdayValue(new Date().getDay())],
      userId: "",
    },
    settings: {
      multipleInstancesPolicy: "ignoreNew",
      disallowStartIfOnBatteries: false,
      stopIfGoingOnBatteries: false,
      allowHardTerminate: true,
      startWhenAvailable: true,
      runOnlyIfNetworkAvailable: false,
      stopOnIdleEnd: false,
      restartOnIdle: false,
      allowStartOnDemand: true,
      hidden: false,
      runOnlyIfIdle: false,
      disallowStartOnRemoteAppSession: false,
      useUnifiedSchedulingEngine: true,
      wakeToRun: false,
      executionTimeLimit: "PT0S",
      priority: 7,
      restartInterval: "PT1M",
      restartCount: 0,
    },
  }
}

export function cloneScheduledTask(task: ScheduledTaskEdit): ScheduledTaskEdit {
  const clone = structuredClone(task)
  clone.trigger.daysOfWeek ??= []
  clone.trigger.interval ||= 1
  clone.trigger.userId ??= ""
  clone.settings ??= createEmptyScheduledTask().settings
  return clone
}

export function validateScheduledTask(task: ScheduledTaskEdit): string {
  if (!task.name.trim()) return "任务名称不能为空"
  if (!task.action.command.trim()) return "程序路径不能为空"
  if (["once", "daily", "weekly"].includes(task.trigger.type) && !task.trigger.startBoundary) {
    return "请选择触发时间"
  }
  if (task.trigger.type === "weekly" && !(task.trigger.daysOfWeek?.length)) {
    return "每周任务至少选择一天"
  }
  if (parseTaskDurationSeconds(task.settings.executionTimeLimit) === null) {
    return "执行时间限制无效"
  }
  if (task.settings.restartCount > 0) {
    const restartSeconds = parseTaskDurationSeconds(task.settings.restartInterval)
    if (restartSeconds === null || restartSeconds <= 0) return "失败重启间隔必须大于 0"
  }
  return ""
}

export function parseTaskDurationSeconds(value: string): number | null {
  const match = value.trim().match(/^P(?:(\d+(?:\.\d+)?)D)?(?:T(?:(\d+(?:\.\d+)?)H)?(?:(\d+(?:\.\d+)?)M)?(?:(\d+(?:\.\d+)?)S)?)?$/i)
  if (!match || !match.slice(1).some((part) => part !== undefined)) return null
  const days = Number(match[1] ?? 0)
  const hours = Number(match[2] ?? 0)
  const minutes = Number(match[3] ?? 0)
  const seconds = Number(match[4] ?? 0)
  const total = days * DURATION_UNIT_SECONDS.days
    + hours * DURATION_UNIT_SECONDS.hours
    + minutes * DURATION_UNIT_SECONDS.minutes
    + seconds
  return Number.isFinite(total) && total >= 0 ? total : null
}

export function formatTaskDuration(amount: number, unit: DurationUnit): string {
  const value = Math.max(0, Math.floor(amount))
  if (value === 0) return "PT0S"
  switch (unit) {
    case "days":
      return `P${value}D`
    case "hours":
      return `PT${value}H`
    case "minutes":
      return `PT${value}M`
    default:
      return `PT${value}S`
  }
}

export function formatTaskDateTime(value: string): string {
  if (!value) return "—"
  const match = value.match(/^(\d{4})-(\d{2})-(\d{2})T(\d{2}):(\d{2})/)
  if (!match) return value
  return `${Number(match[2])}/${Number(match[3])} ${match[4]}:${match[5]}`
}

export function stateLabel(state: string): string {
  switch (state) {
    case "running":
      return "运行中"
    case "queued":
      return "排队中"
    case "disabled":
      return "已禁用"
    default:
      return ""
  }
}

export function defaultStartBoundary(): string {
  const date = new Date(Date.now() + 5 * 60_000)
  date.setSeconds(0, 0)
  const pad = (value: number) => String(value).padStart(2, "0")
  return `${date.getFullYear()}-${pad(date.getMonth() + 1)}-${pad(date.getDate())}T${pad(date.getHours())}:${pad(date.getMinutes())}`
}

function weekdayValue(day: number): string {
  return ["sunday", "monday", "tuesday", "wednesday", "thursday", "friday", "saturday"][day]
}

