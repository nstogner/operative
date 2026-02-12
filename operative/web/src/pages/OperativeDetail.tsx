import { useEffect, useState, useRef, useCallback } from 'react';
import { useParams, useNavigate } from 'react-router-dom';
import type { Operative, StreamEntry, Note } from '@/lib/api';
import {
    getOperative, updateOperative,
    connectChat, getStream,
    listNotes, createNote, deleteNote, keywordSearchNotes,
    getSandboxStatus,
} from '@/lib/api';
import { Button } from '@/components/ui/button';
import { Input } from '@/components/ui/input';
import { Textarea } from '@/components/ui/textarea';
import { Tabs, TabsContent, TabsList, TabsTrigger } from '@/components/ui/tabs';
import { Card, CardContent, CardHeader, CardTitle } from '@/components/ui/card';
import { ScrollArea } from '@/components/ui/scroll-area';
import { Separator } from '@/components/ui/separator';
import { Badge } from '@/components/ui/badge';

export function OperativeDetail() {
    const { id } = useParams<{ id: string }>();
    const navigate = useNavigate();
    const [operative, setOperative] = useState<Operative | null>(null);
    const [entries, setEntries] = useState<StreamEntry[]>([]);
    const [notes, setNotes] = useState<Note[]>([]);
    const [message, setMessage] = useState('');
    const [editInstructions, setEditInstructions] = useState('');
    const [noteTitle, setNoteTitle] = useState('');
    const [noteContent, setNoteContent] = useState('');
    const [searchQuery, setSearchQuery] = useState('');
    const [searchResults, setSearchResults] = useState<Note[] | null>(null);
    const wsRef = useRef<WebSocket | null>(null);
    const scrollRef = useRef<HTMLDivElement>(null);
    const [activeTab, setActiveTab] = useState('chat');
    const [sandboxStatus, setSandboxStatus] = useState<string>('unknown');

    const loadOperative = useCallback(async () => {
        if (!id) return;
        const op = await getOperative(id);
        setOperative(op);
        setEditInstructions(op.admin_instructions);
    }, [id]);

    const loadNotes = useCallback(async () => {
        if (!id) return;
        const n = await listNotes(id);
        setNotes(n || []);
    }, [id]);

    useEffect(() => {
        loadOperative();
        loadNotes();
    }, [loadOperative, loadNotes]);

    // Poll sandbox status every 3s until running.
    useEffect(() => {
        if (!id) return;
        let cancelled = false;

        const poll = async () => {
            try {
                const res = await getSandboxStatus(id);
                if (!cancelled) setSandboxStatus(res.status);
            } catch {
                if (!cancelled) setSandboxStatus('unknown');
            }
        };

        poll();
        const interval = setInterval(poll, 3000);
        return () => { cancelled = true; clearInterval(interval); };
    }, [id]);

    // WebSocket connection
    useEffect(() => {
        if (!id) return;

        const ws = connectChat(id);
        wsRef.current = ws;

        ws.onmessage = (event) => {
            const entry: StreamEntry = JSON.parse(event.data);
            setEntries((prev) => {
                if (prev.some((e) => e.id === entry.id)) return prev;
                // When a compaction_summary arrives, discard all prior entries
                // and start the view from the compaction entry.
                if (entry.role === 'compaction_summary') {
                    return [entry];
                }
                return [...prev, entry];
            });
        };

        ws.onclose = () => {
            // Fallback: load entries via REST
            getStream(id).then((e) => setEntries(e || []));
        };

        return () => {
            ws.close();
        };
    }, [id]);

    // Auto-scroll
    useEffect(() => {
        scrollRef.current?.scrollIntoView({ behavior: 'smooth' });
    }, [entries]);

    const sandboxReady = sandboxStatus === 'running';

    const sendMessage = () => {
        if (!message.trim() || !wsRef.current || !sandboxReady) return;
        wsRef.current.send(JSON.stringify({ content: message }));
        setMessage('');
    };

    const handleKeyDown = (e: React.KeyboardEvent) => {
        if (e.key === 'Enter' && !e.shiftKey) {
            e.preventDefault();
            sendMessage();
        }
    };

    const saveInstructions = async () => {
        if (!id || !operative) return;
        await updateOperative(id, { ...operative, admin_instructions: editInstructions });
        loadOperative();
    };

    const handleCreateNote = async () => {
        if (!id || !noteTitle.trim()) return;
        await createNote(id, { title: noteTitle, content: noteContent });
        setNoteTitle('');
        setNoteContent('');
        loadNotes();
    };

    const handleDeleteNote = async (noteId: string) => {
        await deleteNote(noteId);
        loadNotes();
    };

    const handleSearch = async () => {
        if (!id || !searchQuery.trim()) return;
        const results = await keywordSearchNotes(id, searchQuery);
        setSearchResults(results);
    };

    if (!operative) {
        return <div className="flex items-center justify-center min-h-screen text-muted-foreground">Loading...</div>;
    }

    return (
        <div className="min-h-screen bg-background">
            <div className="max-w-6xl mx-auto p-6 space-y-4">
                {/* Header */}
                <div className="flex items-center justify-between">
                    <div className="flex items-center gap-3">
                        <Button variant="ghost" onClick={() => navigate('/operatives')}>← Back</Button>
                        <h1 className="text-2xl font-bold">{operative.name}</h1>
                        <Badge variant="outline">{operative.model}</Badge>
                    </div>
                </div>

                <Tabs value={activeTab} onValueChange={setActiveTab} className="w-full">
                    <TabsList className="grid w-full grid-cols-3">
                        <TabsTrigger value="chat">Chat</TabsTrigger>
                        <TabsTrigger value="config">Config</TabsTrigger>
                        <TabsTrigger value="notes">Notes</TabsTrigger>
                    </TabsList>

                    {/* Chat Tab */}
                    <TabsContent value="chat" className="mt-4">
                        <Card className="h-[calc(100vh-16rem)]">
                            <CardContent className="flex flex-col h-full p-0">
                                {!sandboxReady && (
                                    <div className="px-4 py-3 bg-amber-500/10 border-b border-amber-500/20 flex items-center gap-2">
                                        <div className="h-2 w-2 rounded-full bg-amber-500 animate-pulse" />
                                        <span className="text-sm text-amber-600 dark:text-amber-400">
                                            Sandbox starting… Chat will be available once the sandbox is running.
                                        </span>
                                    </div>
                                )}
                                <ScrollArea className="flex-1 p-4">
                                    <div className="space-y-4">
                                        {entries.map((entry) => (
                                            <MessageBubble key={entry.id} entry={entry} />
                                        ))}
                                        <div ref={scrollRef} />
                                    </div>
                                </ScrollArea>
                                <Separator />
                                <div className="p-4 flex gap-2">
                                    <Textarea
                                        value={message}
                                        onChange={(e) => setMessage(e.target.value)}
                                        onKeyDown={handleKeyDown}
                                        placeholder={sandboxReady ? 'Type a message...' : 'Waiting for sandbox...'}
                                        className="min-h-[44px] max-h-32 resize-none"
                                        rows={1}
                                        disabled={!sandboxReady}
                                    />
                                    <Button onClick={sendMessage} size="lg" disabled={!sandboxReady}>Send</Button>
                                </div>
                            </CardContent>
                        </Card>
                    </TabsContent>

                    {/* Config Tab */}
                    <TabsContent value="config" className="mt-4">
                        <Card>
                            <CardHeader>
                                <CardTitle>Operative Configuration</CardTitle>
                            </CardHeader>
                            <CardContent className="space-y-4">
                                <div>
                                    <label className="text-sm font-medium">Model</label>
                                    <Input value={operative.model} disabled />
                                </div>
                                <div>
                                    <label className="text-sm font-medium">Admin Instructions</label>
                                    <Textarea
                                        value={editInstructions}
                                        onChange={(e) => setEditInstructions(e.target.value)}
                                        rows={8}
                                    />
                                </div>
                                <div>
                                    <label className="text-sm font-medium">Operative Self-Set Instructions</label>
                                    <Textarea value={operative.operative_instructions} disabled rows={4} />
                                </div>
                                <Button onClick={saveInstructions}>Save Instructions</Button>
                            </CardContent>
                        </Card>
                    </TabsContent>

                    {/* Notes Tab */}
                    <TabsContent value="notes" className="mt-4">
                        <div className="grid gap-4 lg:grid-cols-2">
                            <Card>
                                <CardHeader>
                                    <CardTitle>Create Note</CardTitle>
                                </CardHeader>
                                <CardContent className="space-y-3">
                                    <Input
                                        value={noteTitle}
                                        onChange={(e) => setNoteTitle(e.target.value)}
                                        placeholder="Note title"
                                    />
                                    <Textarea
                                        value={noteContent}
                                        onChange={(e) => setNoteContent(e.target.value)}
                                        placeholder="Note content"
                                        rows={4}
                                    />
                                    <Button onClick={handleCreateNote}>Save Note</Button>
                                </CardContent>
                            </Card>

                            <Card>
                                <CardHeader>
                                    <CardTitle>Search Notes</CardTitle>
                                </CardHeader>
                                <CardContent className="space-y-3">
                                    <div className="flex gap-2">
                                        <Input
                                            value={searchQuery}
                                            onChange={(e) => setSearchQuery(e.target.value)}
                                            placeholder="Search keywords..."
                                            onKeyDown={(e) => e.key === 'Enter' && handleSearch()}
                                        />
                                        <Button onClick={handleSearch}>Search</Button>
                                    </div>
                                    {searchResults && (
                                        <div className="space-y-2">
                                            {searchResults.length === 0 && (
                                                <p className="text-sm text-muted-foreground">No results found.</p>
                                            )}
                                            {searchResults.map((note) => (
                                                <Card key={note.id}>
                                                    <CardContent className="p-3">
                                                        <p className="font-medium text-sm">{note.title}</p>
                                                        <p className="text-xs text-muted-foreground line-clamp-2">{note.content}</p>
                                                    </CardContent>
                                                </Card>
                                            ))}
                                        </div>
                                    )}
                                </CardContent>
                            </Card>

                            <Card className="lg:col-span-2">
                                <CardHeader>
                                    <CardTitle>All Notes ({notes.length})</CardTitle>
                                </CardHeader>
                                <CardContent>
                                    {notes.length === 0 ? (
                                        <p className="text-muted-foreground text-sm">No notes yet.</p>
                                    ) : (
                                        <div className="space-y-2">
                                            {notes.map((note) => (
                                                <div key={note.id} className="flex items-start justify-between p-3 rounded-lg border">
                                                    <div>
                                                        <p className="font-medium text-sm">{note.title}</p>
                                                        <p className="text-xs text-muted-foreground mt-1 line-clamp-2">{note.content}</p>
                                                    </div>
                                                    <Button
                                                        variant="ghost"
                                                        size="sm"
                                                        onClick={() => handleDeleteNote(note.id)}
                                                    >
                                                        ✕
                                                    </Button>
                                                </div>
                                            ))}
                                        </div>
                                    )}
                                </CardContent>
                            </Card>
                        </div>
                    </TabsContent>
                </Tabs>
            </div>
        </div>
    );
}

function MessageBubble({ entry }: { entry: StreamEntry }) {
    const isUser = entry.role === 'user';
    const isCompaction = entry.role === 'compaction_summary';
    const isSystem = entry.role === 'system';
    const isTool = entry.role === 'tool';
    const isToolCall = entry.content_type === 'tool_call';
    const isToolResult = entry.content_type === 'tool_result';

    let content = entry.content;
    let toolInfo: { name?: string; id?: string } | null = null;

    // Parse tool call/result JSON
    if (isToolCall) {
        try {
            const tc = JSON.parse(content);
            toolInfo = { name: tc.name, id: tc.id };
            content = tc.input?.code || tc.input?.instructions || tc.input?.query ||
                tc.input?.title || tc.input?.content || JSON.stringify(tc.input, null, 2);
        } catch { /* use raw content */ }
    }

    if (isToolResult) {
        try {
            const tr = JSON.parse(content);
            content = tr.content || JSON.stringify(tr, null, 2);
        } catch { /* use raw content */ }
    }

    if (isCompaction) {
        return (
            <div className="animate-fade-in">
                <div className="rounded-lg border border-dashed border-muted-foreground/30 bg-muted/30 p-3 mx-4">
                    <div className="flex items-center gap-2 mb-1">
                        <Badge variant="secondary" className="text-xs">📋 Compaction Summary</Badge>
                    </div>
                    <p className="text-xs text-muted-foreground whitespace-pre-wrap">{content}</p>
                </div>
            </div>
        );
    }

    if (isSystem) {
        return (
            <div className="text-center">
                <Badge variant="secondary" className="text-xs">{content}</Badge>
            </div>
        );
    }

    return (
        <div className={`flex ${isUser ? 'justify-end' : 'justify-start'}`}>
            <div className={`max-w-[80%] rounded-lg p-3 ${isUser
                ? 'bg-primary text-primary-foreground'
                : isTool || isToolResult
                    ? 'bg-muted border border-dashed'
                    : isToolCall
                        ? 'bg-muted/50 border'
                        : 'bg-muted'
                }`}>
                {toolInfo && (
                    <div className="flex items-center gap-1 mb-1">
                        <Badge variant="outline" className="text-xs">{toolInfo.name}</Badge>
                    </div>
                )}
                {isTool && <Badge variant="outline" className="text-xs mb-1">Tool Result</Badge>}
                <p className="text-sm whitespace-pre-wrap break-words">{content}</p>
                {entry.model && (
                    <p className="text-xs opacity-50 mt-1">{entry.model}</p>
                )}
            </div>
        </div>
    );
}
