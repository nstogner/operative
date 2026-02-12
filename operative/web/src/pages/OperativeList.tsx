import { useEffect, useState, useCallback } from 'react';
import { useNavigate } from 'react-router-dom';
import type { Operative, Model } from '@/lib/api';
import { listOperatives, createOperative, deleteOperative, listModels, getSandboxStatus } from '@/lib/api';
import { Card, CardHeader, CardTitle, CardDescription, CardContent } from '@/components/ui/card';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { Badge } from '@/components/ui/badge';
import { Dialog, DialogContent, DialogHeader, DialogTitle, DialogTrigger } from '@/components/ui/dialog';

export function OperativeList() {
    const navigate = useNavigate();
    const [operatives, setOperatives] = useState<Operative[]>([]);
    const [models, setModels] = useState<Model[]>([]);
    const [sandboxStatuses, setSandboxStatuses] = useState<Record<string, string>>({});
    const [dialogOpen, setDialogOpen] = useState(false);
    const [newName, setNewName] = useState('');
    const [newModel, setNewModel] = useState('');
    const [newInstructions, setNewInstructions] = useState('');

    const loadData = useCallback(async () => {
        const [ops, mods] = await Promise.all([listOperatives(), listModels()]);
        setOperatives(ops || []);
        setModels(mods || []);
        if (mods?.length && !newModel) {
            setNewModel(mods[0].id);
        }

        // Load sandbox statuses
        const statuses: Record<string, string> = {};
        for (const op of ops || []) {
            try {
                const status = await getSandboxStatus(op.id);
                statuses[op.id] = status.status;
            } catch {
                statuses[op.id] = 'unknown';
            }
        }
        setSandboxStatuses(statuses);
    }, [newModel]);

    useEffect(() => { loadData(); }, [loadData]);

    const handleCreate = async () => {
        if (!newName.trim()) return;
        await createOperative({
            name: newName,
            model: newModel,
            admin_instructions: newInstructions,
        });
        setNewName('');
        setNewInstructions('');
        setDialogOpen(false);
        loadData();
    };

    const handleDelete = async (id: string) => {
        if (!confirm('Delete this operative?')) return;
        await deleteOperative(id);
        loadData();
    };

    return (
        <div className="min-h-screen bg-background">
            <div className="max-w-4xl mx-auto p-6 space-y-6">
                <div className="flex items-center justify-between">
                    <div>
                        <h1 className="text-3xl font-bold tracking-tight">Operatives</h1>
                        <p className="text-muted-foreground mt-1">Manage your AI operatives</p>
                    </div>
                    <Dialog open={dialogOpen} onOpenChange={setDialogOpen}>
                        <DialogTrigger asChild>
                            <Button size="lg">+ New Operative</Button>
                        </DialogTrigger>
                        <DialogContent>
                            <DialogHeader>
                                <DialogTitle>Create New Operative</DialogTitle>
                            </DialogHeader>
                            <div className="space-y-4 pt-2">
                                <div>
                                    <label className="text-sm font-medium">Name</label>
                                    <Input
                                        value={newName}
                                        onChange={(e) => setNewName(e.target.value)}
                                        placeholder="My Operative"
                                    />
                                </div>
                                <div>
                                    <label className="text-sm font-medium">Model</label>
                                    <select
                                        className="w-full rounded-md border border-input bg-background px-3 py-2 text-sm"
                                        value={newModel}
                                        onChange={(e) => setNewModel(e.target.value)}
                                    >
                                        {models.map((m) => (
                                            <option key={m.id} value={m.id}>{m.name || m.id}</option>
                                        ))}
                                    </select>
                                </div>
                                <div>
                                    <label className="text-sm font-medium">Instructions</label>
                                    <Textarea
                                        value={newInstructions}
                                        onChange={(e) => setNewInstructions(e.target.value)}
                                        placeholder="System instructions for this operative..."
                                        rows={4}
                                    />
                                </div>
                                <Button onClick={handleCreate} className="w-full">Create</Button>
                            </div>
                        </DialogContent>
                    </Dialog>
                </div>

                {operatives.length === 0 && (
                    <Card>
                        <CardContent className="p-8 text-center text-muted-foreground">
                            No operatives yet. Create one to get started.
                        </CardContent>
                    </Card>
                )}

                <div className="grid gap-4">
                    {operatives.map((op) => (
                        <Card
                            key={op.id}
                            className="cursor-pointer hover:border-primary/50 transition-colors"
                            onClick={() => navigate(`/operatives/${op.id}`)}
                        >
                            <CardHeader className="pb-3">
                                <div className="flex items-center justify-between">
                                    <CardTitle className="text-lg">{op.name}</CardTitle>
                                    <div className="flex items-center gap-2">
                                        <Badge variant={sandboxStatuses[op.id] === 'running' ? 'default' : 'secondary'}>
                                            {sandboxStatuses[op.id] || 'unknown'}
                                        </Badge>
                                        <Badge variant="outline">{op.model}</Badge>
                                        <Button
                                            variant="ghost"
                                            size="sm"
                                            onClick={(e) => { e.stopPropagation(); handleDelete(op.id); }}
                                        >
                                            ✕
                                        </Button>
                                    </div>
                                </div>
                                <CardDescription className="line-clamp-2">
                                    {op.admin_instructions || 'No instructions set'}
                                </CardDescription>
                            </CardHeader>
                        </Card>
                    ))}
                </div>
            </div>
        </div>
    );
}
