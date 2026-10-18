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
