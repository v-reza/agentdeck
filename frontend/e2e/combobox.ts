import { expect, type Locator, type Page } from '@playwright/test'

/** Both a page and a subtree answer `getByRole`, and specs use both. */
type Scope = Page | Locator

/**
 * Driving a `Combobox` from a test.
 *
 * A native `<select>` is one call — `selectOption`. This control is not, and the
 * trap is worth naming: the `Field` wrapper is a `<label>`, so `getByLabel`
 * resolves to whichever *labelable* element sits inside it — the hidden input
 * that carries the value for a closed list, or the real input for a text box.
 * Neither is the thing to click. Both forms expose `role="combobox"`, so that is
 * what these helpers address.
 *
 * Two shapes behind one role:
 *
 *  - a closed list is a `<button role="combobox">` whose text is the choice;
 *  - a list that accepts a value outside it (`allowCustom`, the BYO model) is an
 *    `<input role="combobox">`, so the value can be typed as well as picked.
 */

/** The visible control, addressed by role so the hidden input cannot win. */
function trigger(scope: Scope, label: RegExp | string): Locator {
  return scope.getByRole('combobox', { name: label })
}

/**
 * Types a value the list does not contain, for a combobox with `allowCustom`.
 *
 * A closed list cannot be driven this way — there is nothing to type into — so
 * a test that needs a free-text value must also assert the field really is the
 * open kind, which this does by failing if the input never appears.
 */
export async function typeCustom(scope: Scope, label: RegExp | string, value: string) {
  const control = trigger(scope, label)
  await control.click()
  await control.fill(value)
  // Commit the value and dismiss the popup. Escape here closes only the popup:
  // `Modal` defers to an open combobox (isComboboxPopupOpen), so the dialog
  // survives — that is exactly the behaviour the register form depends on.
  await control.press('Escape')
  await expect(control).toHaveValue(value)
}

export async function chooseOption(scope: Scope, label: RegExp | string, value: string) {
  const control = trigger(scope, label)
  await control.click()

  const option = scope.getByRole('option', { name: value, exact: true }).first()
  if (await option.count()) {
    await option.click()
  } else {
    // A value the list does not offer. Only an `allowCustom` field can take it —
    // that is the point of the case that uses this path (a model outside the
    // catalog must be reachable in order for refusing it to be a rule).
    await control.fill(value)
    await control.click()
  }

  const tag = await control.evaluate((el) => el.tagName)
  if (tag === 'INPUT') {
    await expect(control).toHaveValue(value)
  } else {
    await expect(control).toContainText(value)
  }
}

/**
 * The values a `Combobox` offers, without leaving the popup open.
 *
 * The popup is dismissed by clicking the control again — its own toggle — and
 * NOT with Escape. Escape reaches the dialog's document-level handler too, so it
 * closes the whole modal and every later locator in the test waits forever on an
 * element that is gone. That failure looks like a broken selector, which is what
 * makes it worth naming here.
 */
export async function optionValues(scope: Scope, label: RegExp | string): Promise<string[]> {
  const control = trigger(scope, label)
  await control.click()
  const values = await scope
    .getByRole('option')
    .evaluateAll((options) => options.map((option) => option.textContent?.trim() ?? ''))
  await control.click()
  return values
}
