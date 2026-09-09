import { toast } from "@/components/ui/toast"

export function errorMessage(reason: unknown): string {
  return reason instanceof Error ? reason.message : String(reason)
}

export function isUserCancelled(reason: unknown): boolean {
  const message = errorMessage(reason).trim().toLowerCase()
  return message === "cancelled by user" || message === "canceled by user" || message.includes("cancelled by user") || message.includes("canceled by user")
}

export function showError(reason: unknown, title = "操作失败"): boolean {
  if (isUserCancelled(reason)) return false
  toast.add({
    type: "error",
    title,
    description: errorMessage(reason),
    priority: "high",
  })
  return true
}

export function showErrorMessage(description: string, title = "操作失败"): string {
  return toast.add({
    type: "error",
    title,
    description,
    priority: "high",
  })
}
