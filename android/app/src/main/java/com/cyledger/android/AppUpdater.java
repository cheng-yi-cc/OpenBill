package com.cyledger.android;

import android.app.Activity;
import android.app.PendingIntent;
import android.content.Context;
import android.content.Intent;
import android.content.SharedPreferences;
import android.content.pm.PackageInfo;
import android.content.pm.PackageInstaller;
import android.content.pm.PackageManager;
import android.net.Uri;
import android.os.Build;
import android.os.Handler;
import android.os.Looper;
import android.provider.Settings;
import android.webkit.WebView;
import android.widget.Toast;

import org.json.JSONArray;
import org.json.JSONObject;

import java.io.ByteArrayOutputStream;
import java.io.File;
import java.io.FileInputStream;
import java.io.FileOutputStream;
import java.io.InputStream;
import java.io.OutputStream;
import java.lang.ref.WeakReference;
import java.net.HttpURLConnection;
import java.net.URL;
import java.nio.charset.StandardCharsets;
import java.security.MessageDigest;
import java.util.Locale;
import java.util.concurrent.ExecutorService;
import java.util.concurrent.Executors;
import java.util.concurrent.atomic.AtomicBoolean;
import java.util.regex.Matcher;
import java.util.regex.Pattern;

final class AppUpdater {
    private static final String LATEST_RELEASE_API = "https://api.github.com/repos/cheng-yi-cc/OpenBill/releases/latest";
    private static final String RELEASES_PAGE_URL = "https://github.com/cheng-yi-cc/OpenBill/releases";
    private static final String APK_ASSET_NAME = "OpenBill-Android-arm64.apk";
    private static final String SHA_ASSET_NAME = "OpenBill-Android-arm64.apk.sha256";
    private static final long SILENT_CHECK_INTERVAL_MS = 24L * 60L * 60L * 1000L;
    private static final int MAX_METADATA_BYTES = 512 * 1024;
    private static final int MAX_SHA_BYTES = 4096;
    private static final long MAX_APK_BYTES = 256L * 1024L * 1024L;
    private static final Pattern SEMVER = Pattern.compile("^v?(\\d+)\\.(\\d+)\\.(\\d+)(?:-([0-9A-Za-z.-]+))?(?:\\+.*)?$");
    private static final Pattern SHA256_LINE = Pattern.compile("^([0-9a-fA-F]{64})(?:\\s|$)");

    private static volatile WeakReference<AppUpdater> activeInstance = new WeakReference<>(null);

    private final MainActivity activity;
    private final SharedPreferences prefs;
    private final Handler handler = new Handler(Looper.getMainLooper());
    private final ExecutorService io = Executors.newSingleThreadExecutor();
    private final AtomicBoolean cancelled = new AtomicBoolean(false);
    private WebView web;

    private String status = "idle";
    private boolean hasUpdate;
    private String latestTag = "";
    private String latestVersion = "";
    private String releaseName = "";
    private String releaseNotes = "";
    private String publishedAt = "";
    private String releaseUrl = RELEASES_PAGE_URL;
    private String apkUrl = "";
    private String shaUrl = "";
    private long apkSize;
    private long downloadedBytes;
    private int progress;
    private String errorMessage = "";
    private File pendingInstallApk;
    private long lastProgressDispatchAt;
    private int lastProgressPercent = -1;

    AppUpdater(MainActivity activity, SharedPreferences prefs) {
        this.activity = activity;
        this.prefs = prefs;
        activeInstance = new WeakReference<>(this);
        restoreCachedRelease();
    }

    void attach(WebView view) {
        this.web = view;
    }

    private void restoreCachedRelease() {
        String tag = prefs.getString("update_latest_tag", "");
        String ver = prefs.getString("update_latest_version", "");
        String apk = prefs.getString("update_apk_url", "");
        String sha = prefs.getString("update_sha_url", "");
        if (tag.isEmpty() || ver.isEmpty() || apk.isEmpty() || sha.isEmpty()) return;
        String current = currentVersionName();
        if (compareSemVer(ver, current) > 0) {
            this.hasUpdate = true;
            this.status = "available";
            this.latestTag = tag;
            this.latestVersion = ver;
            this.releaseName = prefs.getString("update_release_name", tag);
            this.releaseNotes = prefs.getString("update_release_notes", "");
            this.publishedAt = prefs.getString("update_published_at", "");
            this.releaseUrl = prefs.getString("update_release_url", RELEASES_PAGE_URL);
            this.apkUrl = apk;
            this.shaUrl = sha;
            this.apkSize = prefs.getLong("update_apk_size", 0L);
        } else {
            clearCachedRelease();
        }
    }

    private void clearCachedRelease() {
        prefs.edit()
            .remove("update_latest_tag")
            .remove("update_latest_version")
            .remove("update_release_name")
            .remove("update_release_notes")
            .remove("update_published_at")
            .remove("update_release_url")
            .remove("update_apk_url")
            .remove("update_sha_url")
            .remove("update_apk_size")
            .apply();
    }

    private PackageInfo packageInfo() throws Exception {
        PackageManager pm = activity.getPackageManager();
        if (Build.VERSION.SDK_INT >= 33) {
            return pm.getPackageInfo(activity.getPackageName(), PackageManager.PackageInfoFlags.of(0));
        }
        return pm.getPackageInfo(activity.getPackageName(), 0);
    }

    private String currentVersionName() {
        try {
            String name = packageInfo().versionName;
            return name != null && !name.isEmpty() ? name : "0.0.0";
        } catch (Exception ignored) {
            return "0.0.0";
        }
    }

    private long currentVersionCode() {
        try {
            PackageInfo info = packageInfo();
            if (Build.VERSION.SDK_INT >= 28) {
                return info.getLongVersionCode();
            }
            return info.versionCode;
        } catch (Exception ignored) {
            return 0L;
        }
    }

    String appInfoJson() {
        try {
            String pkg = activity.getPackageName();
            return new JSONObject()
                .put("versionName", currentVersionName())
                .put("versionCode", currentVersionCode())
                .put("applicationId", pkg)
                .put("isQa", pkg.endsWith(".qa"))
                .put("isNative", true)
                .toString();
        } catch (Exception e) {
            return "{}";
        }
    }

    synchronized String stateJson() {
        try {
            return new JSONObject()
                .put("status", status)
                .put("hasUpdate", hasUpdate)
                .put("currentVersion", currentVersionName())
                .put("latestTag", latestTag)
                .put("latestVersion", latestVersion)
                .put("releaseName", releaseName)
                .put("releaseNotes", releaseNotes)
                .put("publishedAt", publishedAt)
                .put("releaseUrl", releaseUrl)
                .put("apkSize", apkSize)
                .put("downloadedBytes", downloadedBytes)
                .put("progress", progress)
                .put("error", errorMessage)
                .put("ignoredTag", prefs.getString("update_ignored_tag", ""))
                .put("lastCheckedAt", prefs.getLong("update_last_checked_at", 0L))
                .toString();
        } catch (Exception e) {
            return "{}";
        }
    }

    private void emitState() {
        final String payload = stateJson();
        handler.post(() -> {
            if (web != null && activity.isLocalPage(web.getUrl())) {
                web.evaluateJavascript(
                    "window.dispatchEvent(new CustomEvent('cyledger-update-state',{detail:JSON.parse(" + JSONObject.quote(payload) + ")}))",
                    null
                );
            }
        });
    }

    void check(boolean manual) {
        synchronized (this) {
            if ("checking".equals(status) || "downloading".equals(status) || "verifying".equals(status) || "installing".equals(status)) {
                emitState();
                return;
            }
            long now = System.currentTimeMillis();
            long last = prefs.getLong("update_last_checked_at", 0L);
            if (!manual && last > 0 && now - last < SILENT_CHECK_INTERVAL_MS) {
                emitState();
                return;
            }
            status = "checking";
            errorMessage = "";
        }
        emitState();
        io.execute(() -> performCheck(manual));
    }

    private void performCheck(boolean manual) {
        HttpURLConnection conn = null;
        try {
            conn = openVerifiedHttps(LATEST_RELEASE_API, true);
            int code = conn.getResponseCode();
            if (code == 404) {
                synchronized (this) {
                    prefs.edit().putLong("update_last_checked_at", System.currentTimeMillis()).apply();
                    hasUpdate = false;
                    latestTag = "";
                    latestVersion = "";
                    clearCachedRelease();
                    status = manual ? "up_to_date" : "idle";
                    errorMessage = "";
                }
                emitState();
                return;
            }
            if (code != 200) {
                throw new Exception("GitHub 返回状态码 " + code);
            }
            byte[] bytes;
            try (InputStream in = conn.getInputStream()) {
                bytes = LedgerRuntime.readBounded(in, MAX_METADATA_BYTES);
            }
            JSONObject json = new JSONObject(new String(bytes, StandardCharsets.UTF_8));
            if (json.optBoolean("draft", false) || json.optBoolean("prerelease", false)) {
                synchronized (this) {
                    prefs.edit().putLong("update_last_checked_at", System.currentTimeMillis()).apply();
                    hasUpdate = false;
                    status = manual ? "up_to_date" : "idle";
                    errorMessage = "";
                }
                emitState();
                return;
            }

            String tag = json.optString("tag_name", "").trim();
            String remoteVer = tag.startsWith("v") ? tag.substring(1) : tag;
            String htmlUrl = json.optString("html_url", RELEASES_PAGE_URL).trim();
            if (!isTrustedReleasePageUrl(htmlUrl)) htmlUrl = RELEASES_PAGE_URL;
            String name = json.optString("name", tag).trim();
            String body = json.optString("body", "").trim();
            String pub = json.optString("published_at", "").trim();

            String foundApk = "";
            String foundSha = "";
            long foundSize = 0L;
            JSONArray assets = json.optJSONArray("assets");
            if (assets != null) {
                for (int i = 0; i < assets.length(); i++) {
                    JSONObject asset = assets.optJSONObject(i);
                    if (asset == null) continue;
                    String assetName = asset.optString("name", "");
                    String dl = asset.optString("browser_download_url", "");
                    if (!isTrustedUrl(dl)) continue;
                    if (APK_ASSET_NAME.equals(assetName)) {
                        foundApk = dl;
                        foundSize = asset.optLong("size", 0L);
                    } else if (SHA_ASSET_NAME.equals(assetName)) {
                        foundSha = dl;
                    }
                }
            }

            prefs.edit().putLong("update_last_checked_at", System.currentTimeMillis()).apply();
            String localVer = currentVersionName();
            if (!tag.isEmpty() && compareSemVer(remoteVer, localVer) > 0 && !foundApk.isEmpty() && !foundSha.isEmpty()) {
                synchronized (this) {
                    hasUpdate = true;
                    status = "available";
                    latestTag = tag;
                    latestVersion = remoteVer;
                    releaseName = name.isEmpty() ? tag : name;
                    releaseNotes = body;
                    publishedAt = pub;
                    releaseUrl = htmlUrl;
                    apkUrl = foundApk;
                    shaUrl = foundSha;
                    apkSize = foundSize;
                    downloadedBytes = 0L;
                    progress = 0;
                    errorMessage = "";
                }
                prefs.edit()
                    .putString("update_latest_tag", tag)
                    .putString("update_latest_version", remoteVer)
                    .putString("update_release_name", releaseName)
                    .putString("update_release_notes", body)
                    .putString("update_published_at", pub)
                    .putString("update_release_url", htmlUrl)
                    .putString("update_apk_url", foundApk)
                    .putString("update_sha_url", foundSha)
                    .putLong("update_apk_size", foundSize)
                    .apply();
            } else {
                synchronized (this) {
                    hasUpdate = false;
                    latestTag = "";
                    latestVersion = "";
                    releaseName = "";
                    releaseNotes = "";
                    publishedAt = "";
                    apkUrl = "";
                    shaUrl = "";
                    apkSize = 0L;
                    status = manual ? "up_to_date" : "idle";
                    errorMessage = "";
                }
                clearCachedRelease();
            }
            emitState();
        } catch (Exception e) {
            synchronized (this) {
                if (manual) {
                    status = "error";
                    errorMessage = "检查更新失败，请检查网络连接或稍后重试";
                } else {
                    status = hasUpdate ? "available" : "idle";
                    errorMessage = "";
                }
            }
            emitState();
        } finally {
            if (conn != null) conn.disconnect();
        }
    }

    void downloadAndInstall() {
        final String targetApkUrl;
        final String targetShaUrl;
        final long expectedSize;
        synchronized (this) {
            if ("downloading".equals(status) || "verifying".equals(status) || "installing".equals(status)) {
                emitState();
                return;
            }
            if (!hasUpdate || apkUrl.isEmpty() || shaUrl.isEmpty()) {
                status = "error";
                errorMessage = "未找到可用的安装包，请先检查更新";
                emitState();
                return;
            }
            targetApkUrl = apkUrl;
            targetShaUrl = shaUrl;
            expectedSize = apkSize;
            cancelled.set(false);
            status = "downloading";
            downloadedBytes = 0L;
            progress = 0;
            lastProgressDispatchAt = 0L;
            lastProgressPercent = -1;
            errorMessage = "";
        }
        emitState();
        io.execute(() -> performDownloadAndVerify(targetApkUrl, targetShaUrl, expectedSize));
    }

    private void performDownloadAndVerify(String targetApkUrl, String targetShaUrl, long expectedSize) {
        File dir = updatesDir();
        File partFile = new File(dir, "OpenBill-update.apk.part");
        File finalFile = new File(dir, "OpenBill-update.apk");
        partFile.delete();
        HttpURLConnection shaConn = null;
        HttpURLConnection apkConn = null;
        try {
            shaConn = openVerifiedHttps(targetShaUrl, false);
            if (shaConn.getResponseCode() != 200) {
                throw new Exception("无法下载校验文件（" + shaConn.getResponseCode() + "）");
            }
            String shaText;
            try (InputStream in = shaConn.getInputStream()) {
                shaText = new String(LedgerRuntime.readBounded(in, MAX_SHA_BYTES), StandardCharsets.UTF_8);
            }
            String expectedShaHex = parseSha256Hex(shaText);
            if (expectedShaHex == null) {
                throw new Exception("校验文件格式无效");
            }
            if (cancelled.get()) {
                onDownloadCancelled();
                return;
            }

            apkConn = openVerifiedHttps(targetApkUrl, false);
            if (apkConn.getResponseCode() != 200) {
                throw new Exception("安装包下载失败（" + apkConn.getResponseCode() + "）");
            }
            long totalBytes = apkConn.getContentLengthLong();
            if (totalBytes <= 0) totalBytes = expectedSize;
            if (totalBytes > MAX_APK_BYTES) {
                throw new Exception("安装包大小超出安全限制");
            }

            MessageDigest sha256 = MessageDigest.getInstance("SHA-256");
            long received = 0L;
            byte[] buffer = new byte[32768];
            try (InputStream in = apkConn.getInputStream(); OutputStream out = new FileOutputStream(partFile)) {
                int read;
                while ((read = in.read(buffer)) != -1) {
                    if (cancelled.get()) {
                        onDownloadCancelled();
                        return;
                    }
                    received += read;
                    if (received > MAX_APK_BYTES) {
                        throw new Exception("安装包大小超出安全限制");
                    }
                    sha256.update(buffer, 0, read);
                    out.write(buffer, 0, read);
                    updateDownloadProgress(received, totalBytes);
                }
                out.flush();
            }

            synchronized (this) {
                status = "verifying";
                downloadedBytes = received;
                progress = 100;
            }
            emitState();

            byte[] actualDigest = sha256.digest();
            byte[] expectedDigest = hexToBytes(expectedShaHex);
            if (!MessageDigest.isEqual(actualDigest, expectedDigest)) {
                partFile.delete();
                throw new Exception("安装包完整性校验失败，已取消安装");
            }

            finalFile.delete();
            if (!partFile.renameTo(finalFile)) {
                throw new Exception("无法保存安装包到临时目录");
            }

            PackageInfo archiveInfo = activity.getPackageManager().getPackageArchiveInfo(finalFile.getAbsolutePath(), 0);
            if (archiveInfo == null || archiveInfo.packageName == null) {
                finalFile.delete();
                throw new Exception("下载的文件不是有效的 Android 安装包");
            }
            if (!archiveInfo.packageName.equals(activity.getPackageName())) {
                finalFile.delete();
                throw new Exception("安装包包名（" + archiveInfo.packageName + "）与当前应用不一致，请通过浏览器下载或使用正式版更新");
            }

            handler.post(() -> startSystemInstall(finalFile));
        } catch (Exception e) {
            partFile.delete();
            if (cancelled.get()) {
                onDownloadCancelled();
                return;
            }
            synchronized (this) {
                status = "error";
                errorMessage = e.getMessage() != null && !e.getMessage().isEmpty() ? e.getMessage() : "更新下载失败，请重试";
            }
            emitState();
        } finally {
            if (shaConn != null) shaConn.disconnect();
            if (apkConn != null) apkConn.disconnect();
        }
    }

    private void onDownloadCancelled() {
        new File(updatesDir(), "OpenBill-update.apk.part").delete();
        synchronized (this) {
            status = hasUpdate ? "available" : "idle";
            downloadedBytes = 0L;
            progress = 0;
            errorMessage = "";
        }
        emitState();
    }

    private void updateDownloadProgress(long received, long totalBytes) {
        int pct = totalBytes > 0 ? (int) Math.min(99L, (received * 100L) / totalBytes) : 0;
        long now = System.currentTimeMillis();
        if (pct == lastProgressPercent && now - lastProgressDispatchAt < 220L) {
            return;
        }
        lastProgressPercent = pct;
        lastProgressDispatchAt = now;
        synchronized (this) {
            downloadedBytes = received;
            progress = pct;
        }
        emitState();
    }

    private void startSystemInstall(File apkFile) {
        if (activity.isFinishing() || activity.isDestroyed()) return;
        if (Build.VERSION.SDK_INT >= 26 && !activity.getPackageManager().canRequestPackageInstalls()) {
            synchronized (this) {
                pendingInstallApk = apkFile;
                status = "pending_permission";
                errorMessage = "";
            }
            emitState();
            Toast.makeText(activity, "请允许 OpenBill 安装应用，授权后返回即可继续更新", Toast.LENGTH_LONG).show();
            try {
                Intent permIntent = new Intent(Settings.ACTION_MANAGE_UNKNOWN_APP_SOURCES, Uri.parse("package:" + activity.getPackageName()));
                activity.startActivityForResult(permIntent, LocalBridge.INSTALL_UNKNOWN);
            } catch (Exception e) {
                synchronized (this) {
                    status = "error";
                    errorMessage = "无法打开系统安装授权设置，请手动在应用设置中允许安装未知应用";
                }
                emitState();
            }
            return;
        }
        commitPackageInstallerSession(apkFile);
    }

    void onUnknownSourcesResult() {
        final File apk;
        synchronized (this) {
            apk = pendingInstallApk;
            pendingInstallApk = null;
        }
        if (apk == null || !apk.isFile()) return;
        if (Build.VERSION.SDK_INT >= 26 && !activity.getPackageManager().canRequestPackageInstalls()) {
            synchronized (this) {
                status = "error";
                errorMessage = "尚未开启「允许安装应用」权限，无法调起安装";
            }
            emitState();
            return;
        }
        commitPackageInstallerSession(apk);
    }

    private void commitPackageInstallerSession(File apkFile) {
        synchronized (this) {
            status = "installing";
            errorMessage = "";
        }
        emitState();
        io.execute(() -> {
            PackageInstaller.Session session = null;
            try {
                PackageInstaller pi = activity.getPackageManager().getPackageInstaller();
                PackageInstaller.SessionParams params = new PackageInstaller.SessionParams(PackageInstaller.SessionParams.MODE_FULL_INSTALL);
                params.setSize(apkFile.length());
                if (Build.VERSION.SDK_INT >= 31) {
                    params.setRequireUserAction(PackageInstaller.SessionParams.USER_ACTION_REQUIRED);
                }
                int sessionId = pi.createSession(params);
                session = pi.openSession(sessionId);
                try (InputStream in = new FileInputStream(apkFile); OutputStream out = session.openWrite("base.apk", 0, apkFile.length())) {
                    byte[] buf = new byte[32768];
                    int read;
                    while ((read = in.read(buf)) != -1) {
                        out.write(buf, 0, read);
                    }
                    session.fsync(out);
                }
                Intent callbackIntent = new Intent(activity, UpdateInstallReceiver.class);
                int flags = PendingIntent.FLAG_UPDATE_CURRENT | (Build.VERSION.SDK_INT >= 31 ? PendingIntent.FLAG_MUTABLE : 0);
                PendingIntent pendingIntent = PendingIntent.getBroadcast(activity, sessionId, callbackIntent, flags);
                session.commit(pendingIntent.getIntentSender());
                session.close();
                session = null;
            } catch (Exception e) {
                if (session != null) {
                    try { session.abandon(); } catch (Exception ignored) {}
                }
                synchronized (this) {
                    status = "error";
                    errorMessage = "启动系统安装器失败，可尝试使用浏览器下载安装包";
                }
                emitState();
            }
        });
    }

    static void onInstallPromptShown() {
        AppUpdater inst = activeInstance.get();
        if (inst == null) return;
        synchronized (inst) {
            inst.status = "installing";
        }
        inst.emitState();
    }

    static void onInstallResult(int installStatus, String message) {
        AppUpdater inst = activeInstance.get();
        if (inst == null) return;
        inst.cleanupUpdatesDir();
        synchronized (inst) {
            if (installStatus == PackageInstaller.STATUS_SUCCESS) {
                inst.status = "up_to_date";
                inst.hasUpdate = false;
                inst.errorMessage = "";
            } else if (installStatus == PackageInstaller.STATUS_FAILURE_ABORTED) {
                inst.status = inst.hasUpdate ? "available" : "idle";
                inst.errorMessage = "";
            } else {
                inst.status = "error";
                inst.errorMessage = message == null || message.isEmpty() ? "安装失败，请重试" : message;
            }
        }
        inst.emitState();
    }

    void cancelDownload() {
        cancelled.set(true);
    }

    void ignoreTag(String tag) {
        if (tag == null || tag.trim().isEmpty()) return;
        prefs.edit().putString("update_ignored_tag", tag.trim()).apply();
        emitState();
    }

    void openReleasePage(String url) {
        String target = url != null && isTrustedReleasePageUrl(url.trim()) ? url.trim() : RELEASES_PAGE_URL;
        try {
            activity.startActivity(new Intent(Intent.ACTION_VIEW, Uri.parse(target)));
        } catch (Exception e) {
            Toast.makeText(activity, "没有可用的浏览器", Toast.LENGTH_SHORT).show();
        }
    }

    void cleanupStaleUpdates() {
        cleanupUpdatesDir();
    }

    private File updatesDir() {
        File dir = new File(activity.getCacheDir(), "updates");
        dir.mkdirs();
        return dir;
    }

    private void cleanupUpdatesDir() {
        try {
            File dir = new File(activity.getCacheDir(), "updates");
            File[] files = dir.listFiles();
            if (files != null) {
                for (File f : files) f.delete();
            }
        } catch (Exception ignored) {}
    }

    void destroy() {
        cancelled.set(true);
        io.shutdownNow();
    }

    private HttpURLConnection openVerifiedHttps(String initialUrl, boolean isGithubApi) throws Exception {
        String current = initialUrl;
        for (int hop = 0; hop <= 5; hop++) {
            URL parsed = new URL(current);
            if (!"https".equalsIgnoreCase(parsed.getProtocol()) || !isTrustedHost(parsed.getHost())) {
                throw new Exception("不受信任的下载地址");
            }
            HttpURLConnection conn = (HttpURLConnection) parsed.openConnection();
            conn.setInstanceFollowRedirects(false);
            conn.setConnectTimeout(12000);
            conn.setReadTimeout(25000);
            conn.setRequestProperty("User-Agent", "OpenBill-Android/" + currentVersionName());
            if (isGithubApi && "api.github.com".equalsIgnoreCase(parsed.getHost())) {
                conn.setRequestProperty("Accept", "application/vnd.github+json");
            }
            int code = conn.getResponseCode();
            if (code == 301 || code == 302 || code == 303 || code == 307 || code == 308) {
                String location = conn.getHeaderField("Location");
                conn.disconnect();
                if (location == null || location.isEmpty()) {
                    throw new Exception("重定向地址无效");
                }
                current = new URL(parsed, location).toString();
                continue;
            }
            return conn;
        }
        throw new Exception("下载重定向次数过多");
    }

    private static boolean isTrustedHost(String host) {
        if (host == null) return false;
        String lower = host.toLowerCase(Locale.ROOT);
        return "api.github.com".equals(lower)
            || "github.com".equals(lower)
            || lower.endsWith(".githubusercontent.com");
    }

    private static boolean isTrustedUrl(String url) {
        if (url == null || url.isEmpty()) return false;
        try {
            URL parsed = new URL(url);
            return "https".equalsIgnoreCase(parsed.getProtocol()) && isTrustedHost(parsed.getHost());
        } catch (Exception ignored) {
            return false;
        }
    }

    private static boolean isTrustedReleasePageUrl(String url) {
        return url != null && (url.equals(RELEASES_PAGE_URL) || url.startsWith(RELEASES_PAGE_URL + "/"));
    }

    static String parseSha256Hex(String content) {
        if (content == null) return null;
        Matcher matcher = SHA256_LINE.matcher(content.trim());
        if (!matcher.find()) return null;
        return matcher.group(1).toLowerCase(Locale.ROOT);
    }

    private static byte[] hexToBytes(String hex) {
        int len = hex.length();
        byte[] data = new byte[len / 2];
        for (int i = 0; i < len; i += 2) {
            data[i / 2] = (byte) ((Character.digit(hex.charAt(i), 16) << 4) + Character.digit(hex.charAt(i + 1), 16));
        }
        return data;
    }

    static int compareSemVer(String remoteRaw, String localRaw) {
        int[][] parsed = { parseVerCore(remoteRaw), parseVerCore(localRaw) };
        String[] pre = { parsePreRelease(remoteRaw), parsePreRelease(localRaw) };
        if (parsed[0] == null || parsed[1] == null) return 0;
        for (int i = 0; i < 3; i++) {
            if (parsed[0][i] != parsed[1][i]) {
                return Integer.compare(parsed[0][i], parsed[1][i]);
            }
        }
        boolean rPre = !pre[0].isEmpty();
        boolean lPre = !pre[1].isEmpty();
        if (!rPre && !lPre) return 0;
        if (!rPre) return 1;
        if (!lPre) return -1;
        String[] ra = pre[0].split("\\.");
        String[] la = pre[1].split("\\.");
        int max = Math.max(ra.length, la.length);
        for (int i = 0; i < max; i++) {
            if (i >= ra.length) return -1;
            if (i >= la.length) return 1;
            boolean rNum = ra[i].matches("\\d+");
            boolean lNum = la[i].matches("\\d+");
            if (rNum && lNum) {
                int cmp = Long.compare(Long.parseLong(ra[i]), Long.parseLong(la[i]));
                if (cmp != 0) return cmp;
            } else if (rNum != lNum) {
                return rNum ? -1 : 1;
            } else {
                int cmp = ra[i].compareTo(la[i]);
                if (cmp != 0) return cmp;
            }
        }
        return 0;
    }

    private static int[] parseVerCore(String raw) {
        if (raw == null) return null;
        Matcher m = SEMVER.matcher(raw.trim());
        if (!m.matches()) return null;
        return new int[] {
            Integer.parseInt(m.group(1)),
            Integer.parseInt(m.group(2)),
            Integer.parseInt(m.group(3))
        };
    }

    private static String parsePreRelease(String raw) {
        if (raw == null) return "";
        Matcher m = SEMVER.matcher(raw.trim());
        if (!m.matches() || m.group(4) == null) return "";
        return m.group(4);
    }
}
