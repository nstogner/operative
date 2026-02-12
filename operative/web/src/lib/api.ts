// API client for the operative backend.

const BASE_URL = '/api';

export interface Operative {
    id: string;
    name: string;
    admin_instructions: string;
    operative_instructions: string;
    model: string;
    compaction_model: string;
    compaction_threshold: number;
    created_at: string;
    updated_at: string;
}

export interface StreamEntry {
    id: string;
    operative_id: string;
    role: string;
    content_type: string;
    content: string;
    model: string;
    timestamp: string;
}

export interface Note {
    id: string;
    operative_id: string;
    title: string;
    content: string;
    created_at: string;
    updated_at: string;
}

export interface Model {
    id: string;
    name: string;
    provider: string;
    max_tokens: number;
}

async function fetchJSON<T>(url: string, options?: RequestInit): Promise<T> {
    const res = await fetch(`${BASE_URL}${url}`, {
        headers: { 'Content-Type': 'application/json' },
        ...options,
    });
    if (!res.ok) {
        const err = await res.text();
        throw new Error(err);
    }
    if (res.status === 204) return {} as T;
    return res.json();
}

// Operatives
export const listOperatives = () => fetchJSON<Operative[]>('/operatives');
export const getOperative = (id: string) => fetchJSON<Operative>(`/operatives/${id}`);
export const createOperative = (data: Partial<Operative>) =>
    fetchJSON<Operative>('/operatives', { method: 'POST', body: JSON.stringify(data) });
export const updateOperative = (id: string, data: Partial<Operative>) =>
    fetchJSON<Operative>(`/operatives/${id}`, { method: 'PUT', body: JSON.stringify(data) });
export const deleteOperative = (id: string) =>
    fetchJSON<void>(`/operatives/${id}`, { method: 'DELETE' });

// Stream
export const getStream = (operativeId: string) =>
    fetchJSON<StreamEntry[]>(`/operatives/${operativeId}/stream`);

// Notes
export const listNotes = (operativeId: string) =>
    fetchJSON<Note[]>(`/operatives/${operativeId}/notes`);
export const createNote = (operativeId: string, data: Partial<Note>) =>
    fetchJSON<Note>(`/operatives/${operativeId}/notes`, { method: 'POST', body: JSON.stringify(data) });
export const getNote = (id: string) => fetchJSON<Note>(`/notes/${id}`);
export const updateNote = (id: string, data: Partial<Note>) =>
    fetchJSON<Note>(`/notes/${id}`, { method: 'PUT', body: JSON.stringify(data) });
export const deleteNote = (id: string) =>
    fetchJSON<void>(`/notes/${id}`, { method: 'DELETE' });
export const keywordSearchNotes = (operativeId: string, query: string) =>
    fetchJSON<Note[]>(`/operatives/${operativeId}/notes/keyword-search?q=${encodeURIComponent(query)}`);

// Sandbox
export const getSandboxStatus = (operativeId: string) =>
    fetchJSON<{ status: string }>(`/operatives/${operativeId}/sandbox/status`);

// Models
export const listModels = () => fetchJSON<Model[]>('/models');

// WebSocket
export function connectChat(operativeId: string): WebSocket {
    const proto = window.location.protocol === 'https:' ? 'wss:' : 'ws:';
    const wsUrl = `${proto}//${window.location.host}/api/operatives/${operativeId}/chat`;
    return new WebSocket(wsUrl);
}
