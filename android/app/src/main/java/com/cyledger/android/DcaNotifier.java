package com.cyledger.android;

import android.app.Notification;
import android.app.NotificationChannel;
import android.app.NotificationManager;
import android.app.PendingIntent;
import android.content.Context;
import android.content.Intent;
import android.content.SharedPreferences;
import android.net.Uri;

import org.json.JSONArray;
import org.json.JSONObject;

import java.util.HashSet;
import java.util.Set;

/** Delivers high-importance system notifications for insufficient DCA funds. */
final class DcaNotifier {
    static final String CHANNEL_ID = "dca_balance_alerts";
    private static final String PREFS = "dca_alert_sent";
    private static final int NOTIFICATION_ID = 9200;

    private DcaNotifier() {}

    static String safeRoute(String route) {
        if (route == null) return "";
        if ("/transaction/add?type=3".equals(route)
            || "/investments/plans".equals(route)
            || "/investments/plans?tab=orders".equals(route)
            || "/crypto/dca".equals(route)
            || "/account/credit".equals(route)
            || "/account/debt".equals(route)
            || "/assets/tool?tool=deposits".equals(route)
            || "/calendar/due".equals(route)) {
            return route;
        }
        if (route.startsWith("/crypto/dca?accountId=")) {
            String accountId = route.substring("/crypto/dca?accountId=".length());
            if (accountId.matches("[a-zA-Z0-9_-]{1,64}")) return route;
        }
        return "";
    }

    static void ensureChannel(Context context) {
        NotificationManager manager = context.getSystemService(NotificationManager.class);
        if (manager != null) ensureChannel(manager);
    }

    private static void ensureChannel(NotificationManager manager) {
        NotificationChannel channel = new NotificationChannel(CHANNEL_ID, "定投余额提醒", NotificationManager.IMPORTANCE_HIGH);
        channel.setDescription("定投余额不足、自动暂停及待处理提醒");
        channel.enableVibration(true);
        channel.enableLights(true);
        channel.setShowBadge(true);
        channel.setLockscreenVisibility(Notification.VISIBILITY_PRIVATE);
        manager.createNotificationChannel(channel);
    }

    static synchronized void apply(Context context, JSONArray alerts) {
        if (alerts == null) return;
        SharedPreferences prefs = context.getSharedPreferences(PREFS, Context.MODE_PRIVATE);
        NotificationManager manager = context.getSystemService(NotificationManager.class);
        if (manager == null) return;

        Set<String> activeIds = new HashSet<>();
        for (int i = 0; i < alerts.length(); i++) {
            JSONObject item = alerts.optJSONObject(i);
            if (item != null) {
                String id = item.optString("id", "");
                if (!id.isEmpty() && id.length() <= 140) activeIds.add(id);
            }
        }

        SharedPreferences.Editor editor = prefs.edit();
        boolean changed = false;
        for (String oldId : new HashSet<>(prefs.getAll().keySet())) {
            if (!activeIds.contains(oldId)) {
                editor.remove(oldId);
                manager.cancel(oldId, NOTIFICATION_ID);
                changed = true;
            }
        }

        if (!manager.areNotificationsEnabled()) {
            if (changed) editor.commit();
            return;
        }

        ensureChannel(manager);
        for (int i = 0; i < alerts.length(); i++) {
            JSONObject item = alerts.optJSONObject(i);
            if (item == null) continue;
            String id = item.optString("id", "");
            String title = item.optString("title", "");
            String text = item.optString("text", "");
            String severity = item.optString("severity", "low");
            String day = item.optString("day", "");
            if (id.isEmpty() || id.length() > 140 || title.isEmpty() || text.isEmpty() || day.isEmpty()) continue;

            String fingerprint = day + ":" + severity;
            if (fingerprint.equals(prefs.getString(id, ""))) continue;

            String route = safeRoute(item.optString("route", ""));
            if (route.isEmpty()) route = "/investments/plans";
            postNotification(context, manager, id, title, text, route);
            editor.putString(id, fingerprint);
            changed = true;
        }
        if (changed) editor.commit();
    }

    static void sendTest(Context context) {
        NotificationManager manager = context.getSystemService(NotificationManager.class);
        if (manager == null || !manager.areNotificationsEnabled()) return;
        ensureChannel(manager);
        postNotification(
            context,
            manager,
            "dca-test-notification",
            "定投余额不足提醒（测试）",
            "示例：余额不足以覆盖近期定投，点击可直达定投页面查看详情",
            "/investments/plans"
        );
    }

    private static int smallIconRes(Context context) {
        int id = context.getResources().getIdentifier("ic_stat_openbill", "drawable", context.getPackageName());
        if (id != 0) return id;
        id = context.getResources().getIdentifier("icon", "drawable", context.getPackageName());
        return id != 0 ? id : android.R.drawable.ic_dialog_info;
    }

    private static void postNotification(Context context, NotificationManager manager, String id, String title, String text, String route) {
        int iconRes = smallIconRes(context);
        Intent open = new Intent(context, MainActivity.class)
            .setData(Uri.parse("cyledger://dca/" + Uri.encode(id)))
            .putExtra("openRoute", route)
            .addFlags(Intent.FLAG_ACTIVITY_CLEAR_TOP | Intent.FLAG_ACTIVITY_SINGLE_TOP);
        PendingIntent content = PendingIntent.getActivity(
            context,
            id.hashCode(),
            open,
            PendingIntent.FLAG_UPDATE_CURRENT | PendingIntent.FLAG_IMMUTABLE
        );
        Notification publicVersion = new Notification.Builder(context, CHANNEL_ID)
            .setSmallIcon(iconRes)
            .setColor(0xFF12786F)
            .setContentTitle("OpenBill")
            .setContentText("有一条定投余额提醒，点击查看")
            .build();
        Notification notification = new Notification.Builder(context, CHANNEL_ID)
            .setSmallIcon(iconRes)
            .setColor(0xFF12786F)
            .setContentTitle(title)
            .setContentText(text)
            .setStyle(new Notification.BigTextStyle().bigText(text))
            .setCategory(Notification.CATEGORY_MESSAGE)
            .setVisibility(Notification.VISIBILITY_PRIVATE)
            .setPublicVersion(publicVersion)
            .setWhen(System.currentTimeMillis())
            .setShowWhen(true)
            .setAutoCancel(true)
            .setContentIntent(content)
            .build();
        manager.notify(id, NOTIFICATION_ID, notification);
    }
}
