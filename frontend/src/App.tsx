import { useState } from "react"
import {
  CalendarClockIcon,
  FileCode2Icon,
  MenuIcon,
  PackageXIcon,
  VariableIcon,
} from "lucide-react"

import {
  Sidebar,
  SidebarContent,
  SidebarGroup,
  SidebarGroupContent,
  SidebarHeader,
  SidebarInset,
  SidebarMenu,
  SidebarMenuButton,
  SidebarMenuItem,
  SidebarProvider,
} from "@/components/ui/sidebar"
import { TooltipProvider } from "@/components/ui/tooltip"
import { Toaster } from "@/components/ui/toast"
import { AppUninstallPage } from "@/features/app-uninstall/app-uninstall-page"
import { ContextMenuPage } from "@/features/context-menu/context-menu-page"
import { EnvironmentVariablesPage } from "@/features/environment-variables/environment-variables-page"
import { FileAssociationPage } from "@/features/file-association/file-association-page"
import { ScheduledTasksPage } from "@/features/scheduled-tasks/scheduled-tasks-page"

type Section = "context-menu" | "file-association" | "scheduled-tasks" | "environment-variables" | "app-uninstall"

function App() {
  const [section, setSection] = useState<Section>("context-menu")
  const [visited, setVisited] = useState<Set<Section>>(() => new Set(["context-menu"]))

  const selectSection = (next: Section) => {
    setVisited((current) => {
      if (current.has(next)) return current
      const updated = new Set(current)
      updated.add(next)
      return updated
    })
    setSection(next)
  }

  return (
    <TooltipProvider>
      <SidebarProvider defaultOpen>
        <Sidebar collapsible="icon">
          <SidebarHeader className="group-data-[collapsible=icon]:hidden">
            <div className="flex h-10 items-center px-2">
              <div className="truncate text-sm font-semibold">Win Admin</div>
            </div>
          </SidebarHeader>

          <SidebarContent>
            <SidebarGroup>
              <SidebarGroupContent>
                <SidebarMenu>
                  <SidebarMenuItem>
                    <SidebarMenuButton
                      isActive={section === "context-menu"}
                      tooltip="上下文菜单"
                      onClick={() => selectSection("context-menu")}
                    >
                      <MenuIcon />
                      <span>上下文菜单</span>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                  <SidebarMenuItem>
                    <SidebarMenuButton
                      isActive={section === "file-association"}
                      tooltip="打开方式"
                      onClick={() => selectSection("file-association")}
                    >
                      <FileCode2Icon />
                      <span>打开方式</span>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                  <SidebarMenuItem>
                    <SidebarMenuButton
                      isActive={section === "scheduled-tasks"}
                      tooltip="计划任务"
                      onClick={() => selectSection("scheduled-tasks")}
                    >
                      <CalendarClockIcon />
                      <span>计划任务</span>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                  <SidebarMenuItem>
                    <SidebarMenuButton
                      isActive={section === "environment-variables"}
                      tooltip="环境变量"
                      onClick={() => selectSection("environment-variables")}
                    >
                      <VariableIcon />
                      <span>环境变量</span>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                  <SidebarMenuItem>
                    <SidebarMenuButton
                      isActive={section === "app-uninstall"}
                      tooltip="应用卸载"
                      onClick={() => selectSection("app-uninstall")}
                    >
                      <PackageXIcon />
                      <span>应用卸载</span>
                    </SidebarMenuButton>
                  </SidebarMenuItem>
                </SidebarMenu>
              </SidebarGroupContent>
            </SidebarGroup>
          </SidebarContent>

        </Sidebar>

        <SidebarInset className="h-svh min-h-0 overflow-hidden">
          <div className="min-h-0 flex-1 overflow-hidden">
            {visited.has("context-menu") ? (
              <div className={section === "context-menu" ? "h-full" : "hidden"}>
                <ContextMenuPage active={section === "context-menu"} />
              </div>
            ) : null}
            {visited.has("file-association") ? (
              <div className={section === "file-association" ? "h-full" : "hidden"}>
                <FileAssociationPage active={section === "file-association"} />
              </div>
            ) : null}
            {visited.has("scheduled-tasks") ? (
              <div className={section === "scheduled-tasks" ? "h-full" : "hidden"}>
                <ScheduledTasksPage active={section === "scheduled-tasks"} />
              </div>
            ) : null}
            {visited.has("environment-variables") ? (
              <div className={section === "environment-variables" ? "h-full" : "hidden"}>
                <EnvironmentVariablesPage active={section === "environment-variables"} />
              </div>
            ) : null}
            {visited.has("app-uninstall") ? (
              <div className={section === "app-uninstall" ? "h-full" : "hidden"}>
                <AppUninstallPage active={section === "app-uninstall"} />
              </div>
            ) : null}
          </div>
        </SidebarInset>
      </SidebarProvider>
      <Toaster />
    </TooltipProvider>
  )
}

export default App

