import { Routes, Route, Navigate } from "react-router-dom"
import { Sidebar } from "./components/Sidebar"
import { AgentList } from "./components/AgentList"
import { SessionList } from "./components/SessionList"
import { ChatWindow } from "./components/ChatWindow"
import { Toaster } from "sonner"
import { useParams } from "react-router-dom"

function Layout() {
    return (
        <div className="flex h-screen w-screen bg-background text-foreground overflow-hidden">
            <Toaster />
            <Sidebar />
            <main className="flex-1 flex overflow-hidden">
                <Routes>
                    <Route path="/" element={<Navigate to="/agents" replace />} />
                    <Route
                        path="/agents"
                        element={
                            <div className="flex-1 overflow-auto">
                                <AgentList />
                            </div>
                        }
                    />
                    <Route
                        path="/sessions/*"
                        element={
                            <>
                                <SessionList />
                                <div className="flex-1 flex flex-col bg-background/50">
                                    <Routes>
                                        <Route path=":id" element={<ChatWindowWrapper />} />
                                        <Route
                                            path=""
                                            element={
                                                <div className="flex-1 flex items-center justify-center text-muted-foreground">
                                                    Select a session to start chatting
                                                </div>
                                            }
                                        />
                                    </Routes>
                                </div>
                            </>
                        }
                    />
                </Routes>
            </main>
        </div>
    )
}

function ChatWindowWrapper() {
    const { id } = useParams()
    if (!id) return null
    return <ChatWindow key={id} sessionId={id} />
}

function App() {
    return <Layout />
}

export default App
