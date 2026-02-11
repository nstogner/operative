import { test, expect } from '@playwright/test';
import fs from 'fs';
import path from 'path';
import { fileURLToPath } from 'url';

const __filename = fileURLToPath(import.meta.url);
const __dirname = path.dirname(__filename);

test('agent lifecycle: create, update, chat, and verify persistence', async ({ page }) => {
    // 1. Load App
    await page.goto('/');
    await expect(page.getByText('Antigravity')).toBeVisible();

    // 2. Nav to Agents
    await page.getByTestId('nav-agents').click();

    // 3. Create Agent
    await page.getByTestId('btn-new-agent').click();

    const uniqueName = `Test Agent ${Date.now()}`;
    await page.getByTestId('input-name').fill(uniqueName);

    // Select Model
    await page.getByTestId('input-model').click();
    // Select a known model from the list to avoid flakiness if "gemini-2.5-flash-image" isn't there
    // But sticking to the previous one as it likely exists or mocks are used? 
    // The previous test used 'models/gemini-2.5-flash-image'.
    // If we want to be safe, we can select the first option or a standard one.
    // Let's try to pick one that usually appears.
    const modelOption = page.getByRole('option').first();
    await modelOption.click();

    await page.getByTestId('input-instructions').fill('You are a test agent.');
    await page.getByTestId('btn-save-agent').click();

    // 4. Verify Agent Created
    await expect(page.getByText(uniqueName)).toBeVisible();

    // 5. Update Agent
    // Use a more robust selector finding the card by text, then the edit button
    // The Card component uses 'rounded-lg' class.
    const agentCard = page.locator('.rounded-lg', { hasText: uniqueName }).first();
    await agentCard.getByTestId(/btn-edit-agent-.*/).click();

    // Wait for the dialog to appear before trying to fill inputs
    await expect(page.getByRole('dialog')).toBeVisible();

    const updatedName = `${uniqueName} Updated`;
    await page.getByTestId('input-name').fill(updatedName);
    await page.getByTestId('btn-save-agent').click();

    // 6. Verify Update
    await expect(page.getByText(updatedName)).toBeVisible();
    await expect(page.getByText(uniqueName, { exact: true })).not.toBeVisible();

    // 7. Chat with Agent
    await page.getByTestId('nav-sessions').click();
    await page.getByTestId('btn-new-session').click();

    // Select the newly created agent (updatedName)
    // The select dropdown might need to be opened first? 
    // The previous test clicked a session item, but here we are creating a NEW session.
    // Wait, step 7 in original test was: Click Nav Sessions -> Click New Session -> Click a session item?
    // No, "Click New Session" usually opens a dialog to START a session.
    // Let's verify the "New Session" flow.
    // In SessionList.tsx: handleCreateClick opens dialog.
    // In NewSessionDialog (assumed): we select an agent and click start.

    // ADJUSTMENT: The previous test code:
    // await page.getByTestId('btn-new-session').click();
    // await page.getByTestId(/session-item-.*/).first().click(); 
    // This implies clicking "New Session" creates it immediately? 
    // OR the previous test was just clicking an EXISTING session?
    // "Select the new session (it should be at the top or we click the first one)"
    // If "New Session" creates it, we are good. 
    // But SessionList.tsx shows a Dialog.
    // Let's assume we need to fill the dialog.

    // However, looking at the previous test, it seemed to skip the dialog interaction???
    // "await page.getByTestId('btn-new-session').click();"
    // "await page.getByTestId(/session-item-.*/).first().click();"

    // If the previous test was clicking "New Session", it opens a dialog.
    // Then it clicks a session item... that implies the session item was ALREADY there or created.
    // I suspect the previous test was flaky or relying on existing state?
    // Let's properly create a session.

    // We are already on the sessions page. 
    // Let's try to just find the new agent in the "New Session" dialog IF it exists.
    // For now, let's stick to the flow of "Create a session for the agent we just made".

    // Flow: 
    // 1. Click + (btn-new-session)
    // 2. Dialog appears.
    // 3. Select Agent.
    // 4. Click Start.

    // We need to implement this correctly.
    // Using the browser subagent findings:
    // "combobox.click()", "select option", "click Start Session"

    // Let's try to implement that.

    // Wait for dialog
    await expect(page.getByRole('dialog')).toBeVisible();

    // Trigger the combobox
    await page.getByRole('combobox').click();

    // Select our agent
    await page.getByRole('option', { name: updatedName }).click();

    // Click Start
    await page.getByRole('button', { name: 'Start Session' }).click();

    // 8. Send Message
    const uniqueMessage = `Hello ${uniqueName} - ${Date.now()}`;
    await page.getByTestId('input-chat').fill(uniqueMessage);
    await page.getByTestId('btn-send').click();

    // 9. Verify Message appears in UI
    await expect(page.getByTestId('msg-user').last()).toContainText(uniqueMessage);

    // 10. Verify Persistence (JSONL)
    // Get Session ID from URL
    await page.waitForTimeout(1000); // Wait for URL update
    const url = page.url();
    const sessionId = url.split('/').pop();
    expect(sessionId).toMatch(/^[0-9a-fA-F-]{36}$/); // UUID regex

    // Construct path to JSONL
    // Assuming tests run from web/tests, store is at ../../store/sessions
    const storePath = path.resolve(__dirname, '../../store/sessions');
    const jsonlPath = path.join(storePath, `${sessionId}.jsonl`);

    // Poll for file existence and content
    let found = false;
    for (let i = 0; i < 10; i++) {
        if (fs.existsSync(jsonlPath)) {
            const content = fs.readFileSync(jsonlPath, 'utf-8');
            if (content.includes(uniqueMessage)) {
                found = true;
                break;
            }
        }
        await page.waitForTimeout(500);
    }
    expect(found).toBe(true);

    // 11. Verify Assistant Response
    await expect(page.getByTestId('msg-assistant')).toBeVisible({ timeout: 30000 });
});
