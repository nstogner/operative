import { useEffect, useState } from "react"
import { Agent } from "../types"
import { Card, CardContent, CardDescription, CardHeader, CardTitle } from "./ui/card"
import { Button } from "./ui/button"
import { Plus, Edit, Trash } from "lucide-react"
import { Dialog, DialogContent, DialogHeader, DialogTitle } from "./ui/dialog"
import { AgentForm } from "./AgentForm"

import { useNavigate } from "react-router-dom"

export function AgentList() {
    const navigate = useNavigate()
    const [agents, setAgents] = useState<Agent[]>([])
    const [isDialogOpen, setIsDialogOpen] = useState(false)
    const [editingAgent, setEditingAgent] = useState<Agent | undefined>(undefined)

    const fetchAgents = () => {
        fetch("/api/agents")
            .then((res) => res.json())
            .then((data) => setAgents(data || []))
    }

    useEffect(() => {
        fetchAgents()
    }, [])

    const handleStartSession = (agentId: string) => {
        fetch("/api/sessions", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify({ agent_id: agentId }),
        })
            .then((res) => res.json())
            .then((data) => {
                if (data.id) {
                    navigate(`/sessions/${data.id}`)
                }
            })
    }

    const handleCreate = (agent: Partial<Agent>) => {
        fetch("/api/agents", {
            method: "POST",
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(agent),
        }).then(() => {
            setIsDialogOpen(false)
            fetchAgents()
        })
    }

    const handleUpdate = (agent: Partial<Agent>) => {
        // Merge ID from editingAgent if missing (shouldn't be)
        const payload = { ...editingAgent, ...agent }
        fetch("/api/agents", {
            method: "POST", // Backend uses POST for create/update
            headers: { "Content-Type": "application/json" },
            body: JSON.stringify(payload),
        }).then(() => {
            setIsDialogOpen(false)
            setEditingAgent(undefined)
            fetchAgents()
        })
    }

    const handleDelete = (id: string) => {
        if (!confirm("Are you sure?")) return
        fetch(`/api/agents/${id}`, { method: "DELETE" }).then(() => fetchAgents())
    }

    const openCreate = () => {
        setEditingAgent(undefined)
        setIsDialogOpen(true)
    }

    const openEdit = (agent: Agent) => {
        setEditingAgent(agent)
        setIsDialogOpen(true)
    }

    return (
        <div className="p-8 space-y-6">
            <div className="flex justify-between items-center">
                <h2 className="text-3xl font-bold tracking-tight">Agents</h2>
                <Button onClick={openCreate} data-testid="btn-new-agent">
                    <Plus className="mr-2 h-4 w-4" /> New Agent
                </Button>
            </div>

            <div className="grid grid-cols-1 md:grid-cols-2 lg:grid-cols-3 gap-6">
                {agents.map((agent) => (
                    <Card key={agent.id} className="hover:bg-muted/50 transition-colors">
                        <CardHeader className="flex flex-row items-center justify-between space-y-0 pb-2">
                            <CardTitle className="text-xl font-bold">{agent.name}</CardTitle>
                        </CardHeader>
                        <CardContent>
                            <CardDescription className="line-clamp-3 mb-4">
                                {agent.instructions}
                            </CardDescription>
                            <div className="text-xs text-muted-foreground mb-4 font-mono">{agent.model}</div>
                            <div className="flex gap-2 justify-end">
                                <Button variant="secondary" size="sm" onClick={() => handleStartSession(agent.id)}>
                                    Chat
                                </Button>
                                <Button variant="outline" size="sm" onClick={() => openEdit(agent)} data-testid={`btn-edit-agent-${agent.id}`}>
                                    <Edit className="h-4 w-4" />
                                </Button>
                                <Button variant="destructive" size="sm" onClick={() => handleDelete(agent.id)}>
                                    <Trash className="h-4 w-4" />
                                </Button>
                            </div>
                        </CardContent>
                    </Card>
                ))}
            </div>

            <Dialog open={isDialogOpen} onOpenChange={setIsDialogOpen}>
                <DialogContent>
                    <DialogHeader>
                        <DialogTitle>{editingAgent ? "Edit Agent" : "Create Agent"}</DialogTitle>
                    </DialogHeader>
                    <AgentForm
                        agent={editingAgent}
                        onSubmit={editingAgent ? handleUpdate : handleCreate}
                        onCancel={() => setIsDialogOpen(false)}
                    />
                </DialogContent>
            </Dialog>
        </div>
    )
}
