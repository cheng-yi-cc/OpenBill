package com.cyledger.android;

import android.app.job.JobParameters;
import android.app.job.JobService;
import android.util.Log;

import org.json.JSONArray;
import org.json.JSONObject;

import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;

/** Runs DCA balance and settlement checks in the background without requiring the UI. */
public final class DcaCheckJobService extends JobService {
    private final ExecutorService executor = Executors.newSingleThreadExecutor();

    @Override
    public boolean onStartJob(JobParameters params) {
        final boolean sync = params.getExtras().getInt("sync", 1) != 0;
        executor.execute(() -> {
            boolean needsReschedule = false;
            try {
                LedgerRuntime.ensureStarted(getApplicationContext());
                if (!LedgerRuntime.waitForReady(getApplicationContext(), 25000L)) {
                    needsReschedule = LedgerRuntime.getStartupError() == null;
                    Log.w("CYLedgerDCA", "backend not ready, reschedule=" + needsReschedule);
                    return;
                }
                String raw = NativeBridge.checkDca(sync);
                if (raw != null && !raw.isEmpty()) {
                    JSONObject payload = new JSONObject(raw);
                    JSONArray alerts = payload.optJSONArray("alerts");
                    JSONArray nextAt = payload.optJSONArray("nextAt");
                    DcaNotifier.apply(getApplicationContext(), alerts);
                    DcaScheduler.scheduleNextAlarms(getApplicationContext(), nextAt);
                }
                DcaScheduler.ensurePeriodic(getApplicationContext());
            } catch (Exception e) {
                Log.w("CYLedgerDCA", "background DCA check failed", e);
                // Next periodic or scheduled run will retry safely.
            } finally {
                jobFinished(params, needsReschedule);
            }
        });
        return true;
    }

    @Override
    public boolean onStopJob(JobParameters params) {
        return true;
    }

    @Override
    public void onDestroy() {
        executor.shutdownNow();
        super.onDestroy();
    }
}
