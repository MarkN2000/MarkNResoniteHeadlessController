export type LogLevel = 'stdout' | 'stderr';

export interface LogEntry {
  id: number;
  timestamp: string;
  level: LogLevel;
  message: string;
}

export class LogBuffer {
  private readonly buffer: (LogEntry | undefined)[];
  private readonly capacity: number;
  private nextEntryId = 1;
  private readonly commandResponses: Map<number, LogEntry[]> = new Map();
  private index = 0;
  private size = 0;

  constructor(capacity: number) {
    this.capacity = capacity;
    this.buffer = new Array(capacity);
  }

  push(level: LogLevel, message: string): LogEntry {
    const entry: LogEntry = {
      id: this.nextEntryId++,
      timestamp: new Date().toISOString(),
      level,
      message
    };
    this.buffer[this.index] = entry;
    this.index = (this.index + 1) % this.capacity;
    if (this.size < this.capacity) {
      this.size += 1;
    }
    this.commandResponses.forEach(entries => entries.push(entry));
    return entry;
  }

  toArray(limit?: number): LogEntry[] {
    const result: LogEntry[] = [];
    const count = limit ? Math.min(limit, this.size) : this.size;
    for (let i = 0; i < count; i += 1) {
      const bufferIndex = (this.index - 1 - i + this.capacity) % this.capacity;
      const entry = this.buffer[bufferIndex];
      if (entry) {
        result.push(entry);
      }
    }
    return result.reverse();
  }

  createResponseCollector(startId: number) {
    const entries: LogEntry[] = [];
    this.commandResponses.set(startId, entries);
    return {
      collect: () => entries.filter(entry => entry.id >= startId),
      dispose: () => this.commandResponses.delete(startId)
    };
  }

  nextId(): number {
    return this.nextEntryId;
  }

  clear(): void {
    this.buffer.fill(undefined);
    this.size = 0;
    this.index = 0;
    this.nextEntryId = 1;
    this.commandResponses.clear();
  }
}
