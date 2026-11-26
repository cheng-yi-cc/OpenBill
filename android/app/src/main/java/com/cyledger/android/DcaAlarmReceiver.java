package com.cyledger.android;

import android.content.BroadcastReceiver;
import android.content.Context;
import android.content.Intent;

/** Wakes up scheduled DCA checks on alarm, boot, app upgrade, or time change. */
public final class DcaAlarmReceiver extends BroadcastReceiver {
    @Override
    public void onReceive(Context context, Intent intent) {
        if (intent == null) return;
        String action = intent.getAction();
        if (Intent.ACTION_BOOT_COMPLETED.equals(action)
            || Intent.ACTION_MY_PACKAGE_REPLACED.equals(action)
            || Intent.ACTION_TIME_CHANGED.equals(action)
            || Intent.ACTION_TIMEZONE_CHANGED.equals(action)
            || DcaScheduler.ACTION_CHECK.equals(action)) {
            DcaScheduler.ensurePeriodic(context);
            DcaScheduler.scheduleImmediate(context, true);
        }
    }
}
