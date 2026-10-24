export function exported(input: string): string {
  return input.trim();
}

export function other(): number {
  return tokenInOtherFunction;
}

describe('cases', () => {
  const cases = [1, 2, 3];
  it('works', () => {});
});

declare global {
  interface Window {
    appState: string;
  }
}

/** Rounds toward zero (truncWord). */
export function documented(): void {}

/** Holds a label. */
export class Holder {
  /** The label. */
  label = 'x';
  run(): void {}
}
