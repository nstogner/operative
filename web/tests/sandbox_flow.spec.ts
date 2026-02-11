import { test, expect } from '@playwright/test';

test('sandbox: can execute python code', async ({ page }) => {
    // 1. Setup Agent
    const agentName = `Sandbox Agent ${Date.now()}`;
    const agentRes = await page.request.post('/api/agents', {
        data: {
            name: agentName,
            model: 'models/gemini-2.5-flash',
            instructions: 'You are a python coding agent. When asked, generate and run python code using the run_ipython_cell tool.'
        }
    });
    const agent = await agentRes.json();

    // 2. Start Session
    const sessionRes = await page.request.post('/api/sessions', {
        data: { agent_id: agent.id }
    });
    const session = await sessionRes.json();

    // 3. Navigate to Session
    await page.goto(`/sessions/${session.id}`);

    // 4. Request Code Execution
    await page.getByTestId('input-chat').fill('Calculate 123 * 456 using python');
    await page.getByTestId('btn-send').click();

    // 5. Verify Tool Invocation
    // The UI shows tool use in a specific way.
    // We expect to see "Tool: run_ipython_cell"
    await expect(page.getByText('Tool: run_ipython_cell')).toBeVisible({ timeout: 30000 });

    // 6. Verify Result
    // We expect "Result (Success)" and the output "56088"
    // With my fix, "Result (Success)" should only appear if it actually worked.
    // We set a very long timeout (130s) because the backend timeout is now 120s.
    await expect(page.getByText('Result (Success)')).toBeVisible({ timeout: 130000 });
    await expect(page.getByText('56088')).toBeVisible();
});
