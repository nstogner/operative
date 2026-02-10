import { useEffect, useState } from "react"
import { SessionInfo } from "../types"
import { ScrollArea } from "./ui/scroll-area"
import { Button } from "./ui/button"
import { Plus } from "lucide-react"

interface SessionListProps {
    onSelectSession: (id: string) => void
    selectedSessionId?: string
}

export function SessionList({ onSelectSession, selectedSessionId }: SessionListProps) {
    const [sessions, setSessions] = useState<SessionInfo[]>([])

    const fetchSessions = () => {
        fetch("/api/sessions")
            .then((res) => res.json())
            .then((data) => setSessions(data || []))
    }

    useEffect(() => {
        fetchSessions()
    }, [])

    const handleCreate = () => {
        // For now, defaulting to "default" agent or prompt user?
        // Let's prompt for Agent ID or just pick first available?
        // Creating session without agent ID might use default.
        // Ideally we select Agent then "Start Session".
        // For now, let's just trigger create with default.
        fetch("/api/sessions", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ agent_id: "" }),
        })
            .then((res) => res.json())
            .then((data) => {
                if (data.id) {
                    onSelectSession(data.id)
                    fetchSessions()
                }
            })
    }

    return (
        <div className="w-64 border-r bg-muted/10 flex flex-col h-full">
            <div className="p-4 border-b flex justify-between items-center">
                <h2 className="font-semibold">History</h2>
                <Button size="icon" variant="ghost" onClick={handleCreate} data-testid="btn-new-session">
                    <Plus className="h-4 w-4" />
                </Button>
            </div>
            <ScrollArea className="flex-1">
                <div className="p-2 space-y-1">
                    {sessions.map((session) => (
                        <Button
                            key={session.ID}
                            variant={selectedSessionId === session.ID ? "secondary" : "ghost"}
                            className="w-full justify-start text-left font-normal"
                            onClick={() => onSelectSession(session.ID)}
                            data-testid={`session-item-${session.ID}`}
                        >
                            <div className="truncate w-full">
                                {session.ID.slice(0, 8)}...
                                <span className="block text-xs text-muted-foreground">
                                    {new Date(session.Modified).toLocaleDateString()}
                                </span>
                            </div>
                        </Button>
                    ))}
                </div>
            </ScrollArea>
        </div>
    )
}
