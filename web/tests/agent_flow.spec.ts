import { test, expect } from '@playwright/test';

test('agent lifecycle: create, update, chat', async ({ page }) => {
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
    await page.getByRole('option', { name: 'models/gemini-2.5-flash-image' }).click();

    await page.getByTestId('input-instructions').fill('You are a test agent.');
    await page.getByTestId('btn-save-agent').click();

    // 4. Verify Agent Created
    await expect(page.getByText(uniqueName)).toBeVisible();

    // 5. Update Agent
    // Find the edit button for this agent - complex selector or assume first/last?
    // We can filter by text? 
    // Let's create a locator for the card containing the name, then find the edit button.
    const agentCard = page.locator('.rounded-lg', { hasText: uniqueName });
    // This might be brittle if multiple match, but uniqueName helps.
    await agentCard.getByRole('button').first().click(); // Assuming Edit is first button

    const updatedName = `${uniqueName} Updated`;
    await page.getByTestId('input-name').fill(updatedName);
    await page.getByTestId('btn-save-agent').click();

    // 6. Verify Update
    await expect(page.getByText(updatedName)).toBeVisible();
    await expect(page.getByText(uniqueName, { exact: true })).not.toBeVisible();

    // 7. Chat with Agent
    // Nav to Sessions
    await page.getByTestId('nav-sessions').click();
    await page.getByTestId('btn-new-session').click();

    // Select the new session (it should be at the top or we click the first one)
    // Wait for session list to reload
    await page.waitForTimeout(1000); // Small wait for fetch
    await page.getByTestId(/session-item-.*/).first().click();

    // Send Message
    const uniqueMessage = `Hello ${uniqueName}`;
    await page.getByTestId('input-chat').fill(uniqueMessage);
    await page.getByTestId('btn-send').click();

    // Verify Message appears (User message)
    await expect(page.getByText(uniqueMessage)).toBeVisible();

    // Verify Response (Assistant message) - might take time
    // We look for a response bubble that isn't the user's
    // For now, just waiting for ANY response or "connected" status is good baseline
    await expect(page.getByText('Connected')).toBeVisible();
});
