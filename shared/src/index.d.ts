export interface ServerStatus {
    running: boolean;
    profile: string | null;
    pid?: number;
}
export interface LogEntry {
    timestamp: string;
    level: 'info' | 'warn' | 'error';
    message: string;
}
export interface ScheduledRestartEntry {
    id: string;
    enabled: boolean;
    type: 'once' | 'weekly' | 'daily';
    specificDate?: {
        year: number;
        month: number;
        day: number;
        hour: number;
        minute: number;
    };
    weeklyDay?: number;
    weeklyTime?: {
        hour: number;
        minute: number;
    };
    dailyTime?: {
        hour: number;
        minute: number;
    };
    configFile: string;
    waitControl?: {
        forceRestartTimeout: number;
        actionTiming: number;
    };
}
export interface RestartConfig {
    triggers: {
        scheduled: {
            enabled: boolean;
            schedules: ScheduledRestartEntry[];
        };
        highLoad: {
            enabled: boolean;
            cpuThreshold: number;
            memoryThreshold: number;
            durationMinutes: number;
        };
        userZero: {
            enabled: boolean;
            minUptimeMinutes: number;
        };
    };
    preRestartActions: {
        waitControl: {
            forceRestartTimeout: number;
            actionTiming: number;
        };
        chatMessage: {
            enabled: boolean;
            message: string;
        };
        itemSpawn: {
            enabled: boolean;
            itemType: string;
            itemUrl: string;
            message: string;
        };
        sessionChanges: {
            setPrivate: boolean;
            setMaxUserToOne: boolean;
            changeSessionName: {
                enabled: boolean;
                newName: string;
            };
        };
    };
    failsafe: {
        retryCount: number;
        retryIntervalSeconds: number;
    };
}
export interface RestartStatus {
    nextScheduledRestart: {
        scheduleId: string | null;
        datetime: string | null;
        configFile: string | null;
    };
    currentUptime: number;
    lastRestart: {
        timestamp: string | null;
        trigger: 'scheduled' | 'highLoad' | 'userZero' | 'manual' | 'forced' | null;
        scheduleId?: string;
    };
    highLoadTriggerDisabledUntil: string | null;
    restartInProgress: boolean;
    waitingForUsers: boolean;
    scheduledRestartPreparing: {
        preparing: boolean;
        scheduleId: string | null;
        scheduledTime: string | null;
        configFile: string | null;
    };
}
//# sourceMappingURL=index.d.ts.map