export type LatestRequestResult<T> =
  | { kind: 'value'; value: T }
  | { kind: 'error'; error: unknown }
  | { kind: 'stale' };

/** Tags async reads so a slower response cannot replace newer page state. */
export class LatestRequestGate {
  private revision = 0;

  begin(): number {
    return ++this.revision;
  }

  invalidate(): void {
    this.revision += 1;
  }

  isCurrent(ticket: number): boolean {
    return ticket === this.revision;
  }

  async settle<T>(ticket: number, request: Promise<T>): Promise<LatestRequestResult<T>> {
    try {
      const value = await request;
      return this.isCurrent(ticket) ? { kind: 'value', value } : { kind: 'stale' };
    } catch (error) {
      return this.isCurrent(ticket) ? { kind: 'error', error } : { kind: 'stale' };
    }
  }
}
