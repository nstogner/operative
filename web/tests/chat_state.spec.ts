import { test, expect } from '@playwright/test';

test('chat state: messages should clear when switching sessions', async ({ page }) => {
    // 1. Setup: Create Agent via API
    const timestamp = Date.now();
    const agentName = `State Test Agent ${timestamp}`;
    const agentRes = await page.request.post('/api/agents', {
        data: {
            name: agentName,
            model: 'models/gemini-2.5-flash',
            instructions: 'You are a test agent.'
        }
    });
    const agent = await agentRes.json();
    const agentId = agent.id;

    // 2. Start Session A via API
    const sessionARes = await page.request.post('/api/sessions', {
        data: { agent_id: agentId }
    });
    const sessionA = await sessionARes.json();

    // 3. Navigate to Session A and Send Message
    await page.goto(`/sessions/${sessionA.id}`);
    const msgA = `Message in Session A ${timestamp}`;
    await page.getByTestId('input-chat').fill(msgA);
    // Wait for websocket to connect or just retry send? 
    // The UI might take a sec to connect WS.
    await page.getByTestId('btn-send').click();
    await expect(page.getByText(msgA)).toBeVisible();

    // 4. Start Session B (Empty) via API
    const sessionBRes = await page.request.post('/api/sessions', {
        data: { agent_id: agentId }
    });
    const sessionB = await sessionBRes.json();

    // 5. Navigate to Session B
    await page.goto(`/sessions/${sessionB.id}`);

    // Wait for navigation and verify empty state
    await expect(page).toHaveURL(new RegExp(`/sessions/${sessionB.id}`));

    // The message from Session A should NOT be visible
    await expect(page.getByText(msgA)).not.toBeVisible();
});
