package com.cyledger.android;

import android.app.AlarmManager;
import android.app.PendingIntent;
import android.app.job.JobInfo;
import android.app.job.JobScheduler;
import android.content.ComponentName;
import android.content.Context;
import android.content.Intent;
import android.net.Uri;
import android.os.PersistableBundle;

import org.json.JSONArray;

/** Schedules background DCA checks via JobScheduler and AlarmManager without a persistent foreground notification. */
final class DcaScheduler {
    static final int JOB_ID_IMMEDIATE = 9101;
    static final int JOB_ID_PERIODIC = 9102;
    static final String ACTION_CHECK = "com.cyledger.android.DCA_CHECK";
    private static final int MAX_ALARMS = 8;

    private DcaScheduler() {}

    static void ensurePeriodic(Context context) {
        JobScheduler scheduler = context.getSystemService(JobScheduler.class);
        if (scheduler == null) return;
        if (scheduler.getPendingJob(JOB_ID_PERIODIC) != null) return;
        PersistableBundle extras = new PersistableBundle();
        extras.putInt("sync", 1);
        JobInfo job = new JobInfo.Builder(JOB_ID_PERIODIC, new ComponentName(context, DcaCheckJobService.class))
            .setPeriodic(60L * 60L * 1000L)
            .setPersisted(true)
            .setRequiredNetworkType(JobInfo.NETWORK_TYPE_NONE)
            .setExtras(extras)
            .build();
        scheduler.schedule(job);
    }

    static void scheduleImmediate(Context context, boolean sync) {
        JobScheduler scheduler = context.getSystemService(JobScheduler.class);
        if (scheduler == null) return;
        PersistableBundle extras = new PersistableBundle();
        extras.putInt("sync", sync ? 1 : 0);
        JobInfo job = new JobInfo.Builder(JOB_ID_IMMEDIATE, new ComponentName(context, DcaCheckJobService.class))
            .setMinimumLatency(0L)
            .setOverrideDeadline(0L)
            .setRequiredNetworkType(JobInfo.NETWORK_TYPE_NONE)
            .setExtras(extras)
            .build();
        scheduler.schedule(job);
    }

    private static PendingIntent alarmIntent(Context context, int slot) {
        Intent intent = new Intent(context, DcaAlarmReceiver.class)
            .setAction(ACTION_CHECK)
            .setData(Uri.parse("cyledger://dca-alarm/" + slot));
        return PendingIntent.getBroadcast(context, 9120 + slot, intent, PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE);
    }

    static void scheduleNextAlarms(Context context, JSONArray nextAt) {
        AlarmManager manager = context.getSystemService(AlarmManager.class);
        if (manager == null) return;
        for (int i = 0; i < MAX_ALARMS; i++) {
            manager.cancel(alarmIntent(context, i));
        }
        if (nextAt == null) return;
        long now = System.currentTimeMillis();
        int slot = 0;
        for (int i = 0; i < nextAt.length() && slot < MAX_ALARMS; i++) {
            long at = nextAt.optLong(i, 0L);
            if (at > now) {
                manager.setAndAllowWhileIdle(AlarmManager.RTC_WAKEUP, at, alarmIntent(context, slot));
                slot++;
            }
        }
    }
}
