import type { Page } from '@playwright/test'

export async function answerConfirmation(page:Page,accept:boolean) {
  await page.getByTestId(accept?'confirm-action':'cancel-action').click()
}
export async function withConfirmation(page:Page,accept:boolean,action:()=>Promise<unknown>) {
  const answer=answerConfirmation(page,accept)
  await action()
  await answer
}
