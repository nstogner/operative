import { useEffect, useState } from "react"
import { SessionInfo } from "../types"
import { ScrollArea } from "./ui/scroll-area"
import { Button } from "./ui/button"
import { Plus, CircleStop } from "lucide-react"
import { Dialog } from "./ui/dialog"
import { NewSessionDialog } from "./NewSessionDialog"
import { toast } from "sonner"

import { useNavigate, useParams } from "react-router-dom"

export function SessionList() {
    const navigate = useNavigate()
    const { id: selectedSessionId } = useParams()
    const [sessions, setSessions] = useState<SessionInfo[]>([])
    const [isDialogOpen, setIsDialogOpen] = useState(false)

    const fetchSessions = () => {
        fetch("/api/sessions")
            .then((res) => res.json())
            .then((data) => setSessions(data || []))
    }

    useEffect(() => {
        fetchSessions()
    }, [])

    const handleCreateClick = () => {
        setIsDialogOpen(true)
    }

    const handleStartSession = (agentId: string) => {
        const promise = fetch("/api/sessions", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ agent_id: agentId }),
        })
            .then((res) => res.json())
            .then((data) => {
                if (data.id) {
                    navigate(`/sessions/${data.id}`)
                    fetchSessions()
                    setIsDialogOpen(false)
                    return data
                }
                throw new Error("Failed to create session")
            })

        toast.promise(promise, {
            loading: "Creating session...",
            success: "Session created!",
            error: "Failed to create session",
        })
    }

    const handleStopSession = (e: React.MouseEvent, sessionId: string) => {
        e.stopPropagation() // Prevent navigation
        const promise = fetch(`/api/sessions/${sessionId}/stop`, {
            method: "POST",
        })
            .then((res) => {
                if (!res.ok) throw new Error("Failed to stop session")
                return res.json()
            })
            .then(() => {
                fetchSessions()
            })

        toast.promise(promise, {
            loading: "Stopping session...",
            success: "Session stopped",
            error: "Failed to stop session",
        })
    }

    return (
        <div className="w-80 border-r bg-muted/10 flex flex-col h-full">
            <div className="p-4 border-b flex justify-between items-center">
                <h2 className="font-semibold">History</h2>
                <Button size="icon" variant="ghost" onClick={handleCreateClick} data-testid="btn-new-session">
                    <Plus className="h-4 w-4" />
                </Button>
            </div>
            <ScrollArea className="flex-1">
                <div className="p-2 space-y-1">
                    {sessions.map((session) => (
                        <div key={session.ID} className="relative group">
                            <Button
                                variant={selectedSessionId === session.ID ? "secondary" : "ghost"}
                                className="w-full justify-start text-left font-normal pr-8"
                                onClick={() => navigate(`/sessions/${session.ID}`)}
                                data-testid={`session-item-${session.ID}`}
                            >
                                <div className="truncate w-full">
                                    <div className="flex items-center gap-2">
                                        <span className="font-semibold block truncate">
                                            {session.AgentName || "Unknown Agent"}
                                        </span>
                                        {session.Status === "active" && (
                                            <span className="h-2 w-2 rounded-full bg-green-500 animate-pulse" />
                                        )}
                                    </div>
                                    <span className="block text-xs text-muted-foreground">
                                        {new Date(session.Modified).toLocaleDateString()}
                                    </span>
                                </div>
                            </Button>
                            {session.Status === "active" && (
                                <Button
                                    size="icon"
                                    variant="ghost"
                                    className="absolute right-1 top-1/2 -translate-y-1/2 h-6 w-6 opacity-0 group-hover:opacity-100 transition-opacity"
                                    onClick={(e) => handleStopSession(e, session.ID)}
                                    title="Stop Session"
                                >
                                    <CircleStop className="h-4 w-4 text-destructive" />
                                </Button>
                            )}
                        </div>
                    ))}
                </div>
            </ScrollArea>

            <Dialog open={isDialogOpen} onOpenChange={setIsDialogOpen}>
                <NewSessionDialog
                    onStartSession={handleStartSession}
                    onCancel={() => setIsDialogOpen(false)}
                />
            </Dialog>
        </div>
    )
}
