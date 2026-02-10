import { Button } from "./ui/button"

import { LayoutDashboard, Users, MessageSquare } from "lucide-react"

interface SidebarProps {
    currentView: "agents" | "sessions"
    onChangeView: (view: "agents" | "sessions") => void
}

export function Sidebar({ currentView, onChangeView }: SidebarProps) {
    return (
        <div className="w-64 border-r bg-muted/20 flex flex-col h-full">
            <div className="p-6">
                <h1 className="text-xl font-bold flex items-center gap-2">
                    <LayoutDashboard className="h-6 w-6" />
                    Antigravity
                </h1>
            </div>
            <div className="px-4 space-y-2">
                <Button
                    variant={currentView === "agents" ? "secondary" : "ghost"}
                    className="w-full justify-start"
                    onClick={() => onChangeView("agents")}
                    data-testid="nav-agents"
                >
                    <Users className="mr-2 h-4 w-4" />
                    Agents
                </Button>
                <Button
                    variant={currentView === "sessions" ? "secondary" : "ghost"}
                    className="w-full justify-start"
                    onClick={() => onChangeView("sessions")}
                    data-testid="nav-sessions"
                >
                    <MessageSquare className="mr-2 h-4 w-4" />
                    Sessions
                </Button>
            </div>
        </div>
    )
}
