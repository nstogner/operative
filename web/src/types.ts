export interface Agent {
    id: string
    name: string
    instructions: string
    model: string
    tools: string[]
}

export interface SessionInfo {
    ID: string
    Path: string
    Status: string
    Created: string
    Modified: string
    // Add other fields as needed
}

export interface Message {
    role: string
    content: Content[]
}

export interface Content {
    type: "text" | "tool_use" | "tool_result"
    text?: {
        content: string
    }
    tool_use?: {
        id: string
        name: string
        input: any
    }
    tool_result?: {
        tool_use_id: string
        is_error: boolean
        content: string
    }
}
