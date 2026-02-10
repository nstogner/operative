import { test, expect } from '@playwright/test';

test('has title', async ({ page }) => {
    await page.goto('/');

    // Expect a title "to contain" a substring.
    await expect(page).toHaveTitle(/Gemini Agent Desktop/);
});

test('loads agents', async ({ page }) => {
    await page.goto('/');
    // Basic check to see if the main layout loads
    await expect(page.locator('text=Antigravity')).toBeVisible();
});
