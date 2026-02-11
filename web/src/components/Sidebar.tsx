import { Button } from "./ui/button"
import { NavLink } from "react-router-dom"
import { LayoutDashboard, Users, MessageSquare } from "lucide-react"
import { cn } from "@/lib/utils"

export function Sidebar() {
    return (
        <div className="w-64 border-r bg-muted/20 flex flex-col h-full">
            <div className="p-6">
                <h1 className="text-xl font-bold flex items-center gap-2">
                    <LayoutDashboard className="h-6 w-6" />
                    Antigravity
                </h1>
            </div>
            <div className="px-4 space-y-2">
                <NavLink
                    to="/agents"
                    className={({ isActive }) =>
                        cn(
                            "w-full justify-start",
                            isActive
                                ? "bg-secondary text-secondary-foreground"
                                : "ghost hover:bg-accent hover:text-accent-foreground"
                        )
                    }
                >
                    {({ isActive }) => (
                        <Button
                            variant={isActive ? "secondary" : "ghost"}
                            className="w-full justify-start"
                            data-testid="nav-agents"
                        >
                            <Users className="mr-2 h-4 w-4" />
                            Agents
                        </Button>
                    )}
                </NavLink>
                <NavLink
                    to="/sessions"
                    className={({ isActive }) =>
                        cn(
                            "w-full justify-start",
                            isActive
                                ? "bg-secondary text-secondary-foreground"
                                : "ghost hover:bg-accent hover:text-accent-foreground"
                        )
                    }
                >
                    {({ isActive }) => (
                        <Button
                            variant={isActive ? "secondary" : "ghost"}
                            className="w-full justify-start"
                            data-testid="nav-sessions"
                        >
                            <MessageSquare className="mr-2 h-4 w-4" />
                            Sessions
                        </Button>
                    )}
                </NavLink>
            </div>
        </div>
    )
}
