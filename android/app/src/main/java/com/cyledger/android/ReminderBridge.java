package com.cyledger.android;

import android.Manifest;
import android.app.Activity;
import android.app.NotificationManager;
import android.content.Context;
import android.content.Intent;
import android.content.SharedPreferences;
import android.content.pm.PackageManager;
import android.net.Uri;
import android.os.Build;
import android.provider.Settings;
import android.webkit.JavascriptInterface;
import android.webkit.WebView;

import org.json.JSONArray;
import org.json.JSONObject;

final class ReminderBridge {
    private final Activity activity;
    private final WebView web;

    ReminderBridge(Activity activity, WebView web) {
        this.activity = activity;
        this.web = web;
    }

    private boolean trusted() {
        Uri uri = Uri.parse(web.getUrl() == null ? "" : web.getUrl());
        int port = LedgerRuntime.port(activity);
        return "http".equals(uri.getScheme()) && "127.0.0.1".equals(uri.getHost()) && uri.getPort() == port;
    }

    private void openNotificationSettings() {
        Intent intent = new Intent(Settings.ACTION_APP_NOTIFICATION_SETTINGS)
            .putExtra(Settings.EXTRA_APP_PACKAGE, activity.getPackageName());
        try {
            activity.startActivity(intent);
        } catch (Exception ignored) {
            activity.startActivity(new Intent(Settings.ACTION_APPLICATION_DETAILS_SETTINGS,
                Uri.parse("package:" + activity.getPackageName())));
        }
    }

    @JavascriptInterface
    public String status() {
        NotificationManager manager = activity.getSystemService(NotificationManager.class);
        return manager != null && manager.areNotificationsEnabled() ? "granted" : "denied";
    }

    @JavascriptInterface
    public void requestPermission() {
        activity.runOnUiThread(() -> {
            if (!trusted()) return;
            NotificationManager manager = activity.getSystemService(NotificationManager.class);
            if (manager != null && manager.areNotificationsEnabled()) return;
            if (Build.VERSION.SDK_INT >= 33
                && activity.checkSelfPermission(Manifest.permission.POST_NOTIFICATIONS) != PackageManager.PERMISSION_GRANTED) {
                SharedPreferences prefs = activity.getSharedPreferences("local_settings", Context.MODE_PRIVATE);
                boolean asked = prefs.getBoolean("askedNotifPerm", false);
                if (!asked || activity.shouldShowRequestPermissionRationale(Manifest.permission.POST_NOTIFICATIONS)) {
                    prefs.edit().putBoolean("askedNotifPerm", true).apply();
                    activity.requestPermissions(new String[]{Manifest.permission.POST_NOTIFICATIONS}, 701);
                    return;
                }
            }
            openNotificationSettings();
        });
    }

    @JavascriptInterface
    public void replace(String json) {
        activity.runOnUiThread(() -> {
            if (trusted()) ReminderReceiver.replace(activity, json);
        });
    }

    @JavascriptInterface
    public void clear() {
        replace("[]");
    }

    @JavascriptInterface
    public void test() {
        activity.runOnUiThread(() -> {
            if (!trusted()) return;
            try {
                JSONArray items = ReminderReceiver.stored(activity);
                items.put(new JSONObject().put("id", "test-notification").put("at", System.currentTimeMillis() + 8000)
                    .put("title", "OpenBill 提醒测试").put("text", "系统到期提醒已正常发送"));
                ReminderReceiver.replace(activity, items.toString());
            } catch (Exception ignored) {}
        });
    }

    @JavascriptInterface
    public void notifyDca(String json) {
        activity.runOnUiThread(() -> {
            if (!trusted()) return;
            try {
                DcaNotifier.apply(activity.getApplicationContext(), new JSONArray(json));
            } catch (Exception ignored) {}
        });
    }

    @JavascriptInterface
    public void scheduleDca(boolean sync) {
        activity.runOnUiThread(() -> {
            if (trusted()) {
                DcaScheduler.ensurePeriodic(activity.getApplicationContext());
                DcaScheduler.scheduleImmediate(activity.getApplicationContext(), sync);
            }
        });
    }

    @JavascriptInterface
    public void testDca() {
        activity.runOnUiThread(() -> {
            if (trusted()) DcaNotifier.sendTest(activity.getApplicationContext());
        });
    }
}
