import { useState, useEffect } from "react"
import { Agent } from "../types"
import { Button } from "./ui/button"
import { Input } from "./ui/input"
import { Textarea } from "./ui/textarea"
import { Label } from "@radix-ui/react-label"
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "./ui/select"

interface AgentFormProps {
    agent?: Agent
    onSubmit: (agent: Partial<Agent>) => void
    onCancel: () => void
}

export function AgentForm({ agent, onSubmit, onCancel }: AgentFormProps) {
    const [formData, setFormData] = useState<Partial<Agent>>(
        agent || {
            name: "",
            instructions: "",
            model: "gemini-1.5-pro-latest", // Default
            tools: [],
        }
    )

    const [models, setModels] = useState<string[]>([])

    useEffect(() => {
        fetch("/api/models")
            .then((res) => res.json())
            .then((data) => setModels(data || []))
            .catch((err) => console.error("Failed to fetch models", err))
    }, [])

    const handleSubmit = (e: React.FormEvent) => {
        e.preventDefault()
        onSubmit(formData)
    }

    return (
        <form onSubmit={handleSubmit} className="space-y-4">
            <div className="space-y-2">
                <Label htmlFor="name">Name</Label>
                <Input
                    id="name"
                    value={formData.name}
                    onChange={(e) => setFormData({ ...formData, name: e.target.value })}
                    required
                    data-testid="input-name"
                />
            </div>
            <div className="space-y-2">
                <Label htmlFor="model">Model</Label>
                <Select
                    value={formData.model}
                    onValueChange={(value) => setFormData({ ...formData, model: value })}
                >
                    <SelectTrigger data-testid="input-model">
                        <SelectValue placeholder="Select a model" />
                    </SelectTrigger>
                    <SelectContent>
                        {models.map((model) => (
                            <SelectItem key={model} value={model}>
                                {model}
                            </SelectItem>
                        ))}
                    </SelectContent>
                </Select>
            </div>
            <div className="space-y-2">
                <Label htmlFor="instructions">Instructions</Label>
                <Textarea
                    id="instructions"
                    value={formData.instructions}
                    onChange={(e) =>
                        setFormData({ ...formData, instructions: e.target.value })
                    }
                    className="h-32"
                    data-testid="input-instructions"
                />
            </div>
            <div className="flex justify-end gap-2 pt-4">
                <Button type="button" variant="outline" onClick={onCancel}>
                    Cancel
                </Button>
                <Button type="submit" data-testid="btn-save-agent">Save Agent</Button>
            </div>
        </form>
    )
}
