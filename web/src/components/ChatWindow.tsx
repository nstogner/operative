import { useEffect, useRef, useState } from "react"
import { Message, Content } from "../types"
import { ScrollArea } from "./ui/scroll-area"
import { Textarea } from "./ui/textarea"
import { Button } from "./ui/button"
import { Send } from "lucide-react"
import { Avatar, AvatarFallback, AvatarImage } from "./ui/avatar"
import ReactMarkdown from "react-markdown"
import { Prism as SyntaxHighlighter } from "react-syntax-highlighter"
import { vscDarkPlus } from "react-syntax-highlighter/dist/esm/styles/prism"

interface ChatWindowProps {
    sessionId: string
}

export function ChatWindow({ sessionId }: ChatWindowProps) {
    const [messages, setMessages] = useState<Message[]>([])
    const [input, setInput] = useState("")
    const [isConnected, setIsConnected] = useState(false)
    const wsRef = useRef<WebSocket | null>(null)
    const scrollRef = useRef<HTMLDivElement>(null)

    // 1. Initial Load (REST)
    useEffect(() => {
        fetch(`/api/sessions/${sessionId}`)
            .then((res) => res.json())
            .then((data) => {
                if (data.entries) {
                    // Transform entries to messages
                    // Backend returns raw entries, we need to filter MessageEntry
                    const history = data.entries
                        .filter((e: any) => e.message)
                        .map((e: any) => e.message)
                    setMessages(history)
                }
            })
    }, [sessionId])

    // 2. WebSocket
    useEffect(() => {
        if (!sessionId) return

        // Clear messages when switching sessions
        setMessages([])

        let ws: WebSocket | null = null
        let timeoutId: NodeJS.Timeout

        const connect = () => {
            // Construct WS URL
            const protocol = window.location.protocol === "https:" ? "wss:" : "ws:"
            const host = window.location.host
            const url = `${protocol}//${host}/api/sessions/${sessionId}/chat`

            ws = new WebSocket(url)
            wsRef.current = ws

            ws.onopen = () => {
                setIsConnected(true)
                // Re-fetch history on reconnect to ensure we didn't miss anything while disconnected
                fetch(`/api/sessions/${sessionId}`)
                    .then((res) => res.json())
                    .then((data) => {
                        if (data.entries) {
                            const history = data.entries
                                .filter((e: any) => e.message)
                                .map((e: any) => e.message)
                            setMessages(history)
                        }
                    })
            }

            ws.onclose = () => {
                setIsConnected(false)
                // Retry connection after 3s
                timeoutId = setTimeout(connect, 3000)
            }

            ws.onmessage = (event) => {
                const entry = JSON.parse(event.data)
                // Check if entry is a message
                if (entry.message) {
                    setMessages((prev) => {
                        // Avoid duplicates if ID matches?
                        // Simple append for now as backend streaming might send diffs or full entries
                        // Backend syncSession sends unchecked entries.
                        // Helper to check duplicates
                        if (prev.find(m => JSON.stringify(m) === JSON.stringify(entry.message))) {
                            return prev
                        }
                        return [...prev, entry.message]
                    })
                }
            }
        }

        connect()

        return () => {
            if (ws) ws.close()
            clearTimeout(timeoutId)
        }
    }, [sessionId])

    useEffect(() => {
        if (scrollRef.current) {
            scrollRef.current.scrollIntoView({ behavior: "smooth" })
        }
    }, [messages])

    const sendMessage = () => {
        if (!input.trim() || !wsRef.current) return
        wsRef.current.send(JSON.stringify({ content: input }))
        setInput("")
    }

    const handleKeyDown = (e: React.KeyboardEvent) => {
        if (e.key === "Enter" && !e.shiftKey) {
            e.preventDefault()
            sendMessage()
        }
    }

    return (
        <div className="flex flex-col h-full">
            <div className="border-b p-4 flex items-center justify-between">
                <h3 className="font-semibold">Session: {sessionId}</h3>
                <span className={`text-xs ${isConnected ? "text-green-500" : "text-red-500"}`}>
                    {isConnected ? "Connected" : "Disconnected"}
                </span>
            </div>

            <ScrollArea className="flex-1 p-4">
                <div className="space-y-4 max-w-3xl mx-auto">
                    {messages.map((msg, i) => (
                        <MessageItem key={i} message={msg} />
                    ))}
                    <div ref={scrollRef} />
                </div>
            </ScrollArea>

            <div className="p-4 border-t bg-background">
                <div className="max-w-3xl mx-auto flex gap-2">
                    <Textarea
                        value={input}
                        onChange={(e) => setInput(e.target.value)}
                        onKeyDown={handleKeyDown}
                        placeholder="Type a message..."
                        className="min-h-[60px]"
                        data-testid="input-chat"
                    />
                    <Button onClick={sendMessage} className="h-auto" data-testid="btn-send">
                        <Send className="h-4 w-4" />
                    </Button>
                </div>
            </div>
        </div>
    )
}

function MessageItem({ message }: { message: Message }) {
    const isUser = message.role === "user"
    return (
        <div className={`flex gap-4 ${isUser ? "flex-row-reverse" : ""}`}>
            <Avatar className="h-8 w-8">
                <AvatarFallback>{isUser ? "U" : "AI"}</AvatarFallback>
                <AvatarImage src={isUser ? "" : "/bot-avatar.png"} />
            </Avatar>
            <div
                className={`flex-1 rounded-lg p-3 ${isUser ? "bg-primary text-primary-foreground" : "bg-muted"}`}
                data-testid={isUser ? "msg-user" : "msg-assistant"}
            >
                {message.content.map((c: Content, i: number) => {
                    if (c.text) {
                        return <ReactMarkdown key={i} className="prose dark:prose-invert break-words">{c.text.content}</ReactMarkdown>
                    }
                    if (c.tool_use) {
                        if (c.tool_use.name === "run_ipython_cell") {
                            const code = c.tool_use.input.code as string
                            return (
                                <div key={i} className="mt-2 rounded-md overflow-hidden border border-border">
                                    <div className="bg-muted px-4 py-2 text-xs font-mono border-b border-border flex items-center justify-between">
                                        <span>Python Cell</span>
                                    </div>
                                    <SyntaxHighlighter
                                        language="python"
                                        style={vscDarkPlus}
                                        customStyle={{ margin: 0, borderRadius: 0 }}
                                    >
                                        {code}
                                    </SyntaxHighlighter>
                                </div>
                            )
                        }
                        return (
                            <div key={i} className="text-xs font-mono bg-black/10 p-2 rounded mt-2">
                                <div>Tool: {c.tool_use.name}</div>
                                <pre>{JSON.stringify(c.tool_use.input, null, 2)}</pre>
                            </div>
                        )
                    }
                    if (c.tool_result) {
                        return (
                            <div key={i} className="text-xs font-mono bg-green-500/10 p-2 rounded mt-2 border border-green-500/20">
                                <div>Result ({c.tool_result.is_error ? "Error" : "Success"})</div>
                                <pre className="whitespace-pre-wrap">{c.tool_result.content}</pre>
                            </div>
                        )
                    }
                    return null
                })}
            </div>
        </div>
    )
}
