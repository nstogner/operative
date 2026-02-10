import { useState } from "react"
import { Sidebar } from "./components/Sidebar"
import { AgentList } from "./components/AgentList"
import { SessionList } from "./components/SessionList"
import { ChatWindow } from "./components/ChatWindow"

function App() {
    const [currentView, setCurrentView] = useState<"agents" | "sessions">("agents")
    const [selectedSessionId, setSelectedSessionId] = useState<string | undefined>()

    return (
        <div className="flex h-screen w-screen bg-background text-foreground overflow-hidden">
            <Sidebar
                currentView={currentView}
                onChangeView={(view) => {
                    setCurrentView(view)
                    if (view === "agents") setSelectedSessionId(undefined)
                }}
            />
            <main className="flex-1 flex overflow-hidden">
                {currentView === "agents" && (
                    <div className="flex-1 overflow-auto">
                        <AgentList />
                    </div>
                )}
                {currentView === "sessions" && (
                    <>
                        <SessionList
                            selectedSessionId={selectedSessionId}
                            onSelectSession={setSelectedSessionId}
                        />
                        <div className="flex-1 flex flex-col bg-background/50">
                            {selectedSessionId ? (
                                <ChatWindow sessionId={selectedSessionId} />
                            ) : (
                                <div className="flex-1 flex items-center justify-center text-muted-foreground">
                                    Select a session to start chatting
                                </div>
                            )}
                        </div>
                    </>
                )}
            </main>
        </div>
    )
}

export default App
