import { useEffect, useState } from "react"
import { Agent } from "../types"
import { DialogContent, DialogHeader, DialogTitle, DialogFooter } from "./ui/dialog"
import { Button } from "./ui/button"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "./ui/select"
import { Loader2 } from "lucide-react"

interface NewSessionDialogProps {
    onStartSession: (agentId: string) => void
    onCancel: () => void
}

export function NewSessionDialog({ onStartSession, onCancel }: NewSessionDialogProps) {
    const [agents, setAgents] = useState<Agent[]>([])
    const [selectedAgentId, setSelectedAgentId] = useState<string>("")
    const [isLoading, setIsLoading] = useState(false)
    const [loadingAgents, setLoadingAgents] = useState(true)

    useEffect(() => {
        fetch("/api/agents")
            .then((res) => res.json())
            .then((data) => {
                setAgents(data || [])
                setLoadingAgents(false)
            })
            .catch(() => setLoadingAgents(false))
    }, [])

    const handleStart = () => {
        console.log("handleStart", { selectedAgentId })
        if (!selectedAgentId) return
        setIsLoading(true)
        onStartSession(selectedAgentId)
    }

    // Debug logging
    useEffect(() => {
        console.log("NewSessionDialog State:", { agentsLength: agents.length, selectedAgentId })
    }, [agents, selectedAgentId])

    return (
        <DialogContent className="sm:max-w-[425px]">
            <DialogHeader>
                <DialogTitle>Start New Session</DialogTitle>
            </DialogHeader>
            <div className="grid gap-4 py-4">
                <div className="space-y-2">
                    <label className="text-sm font-medium leading-none peer-disabled:cursor-not-allowed peer-disabled:opacity-70">
                        Select Agent
                    </label>
                    {loadingAgents ? (
                        <div className="flex items-center space-x-2 text-sm text-muted-foreground">
                            <Loader2 className="h-4 w-4 animate-spin" />
                            <span>Loading agents...</span>
                        </div>
                    ) : (
                        <Select onValueChange={setSelectedAgentId} value={selectedAgentId}>
                            <SelectTrigger>
                                <SelectValue placeholder="Select an agent..." />
                            </SelectTrigger>
                            <SelectContent>
                                {agents.length === 0 ? (
                                    <div className="p-2 text-sm text-muted-foreground text-center">
                                        No agents found. Create one first!
                                    </div>
                                ) : (
                                    agents.map((agent) => (
                                        <SelectItem key={agent.id} value={agent.id}>
                                            {agent.name}
                                        </SelectItem>
                                    ))
                                )}
                            </SelectContent>
                        </Select>
                    )}
                </div>
            </div>
            <DialogFooter>
                <Button variant="outline" onClick={onCancel} disabled={isLoading}>
                    Cancel
                </Button>
                <Button onClick={handleStart} disabled={!selectedAgentId || isLoading} data-testid="btn-start-session">
                    {isLoading && <Loader2 className="mr-2 h-4 w-4 animate-spin" />}
                    Start Session
                </Button>
            </DialogFooter>
        </DialogContent>
    )
}
