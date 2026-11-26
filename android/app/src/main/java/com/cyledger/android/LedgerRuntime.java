package com.cyledger.android;

import android.content.Context;
import android.util.Base64;

import java.io.ByteArrayOutputStream;
import java.io.File;
import java.io.FileInputStream;
import java.io.FileOutputStream;
import java.io.InputStream;
import java.io.OutputStream;
import java.net.HttpURLConnection;
import java.net.URL;
import java.nio.charset.StandardCharsets;
import java.security.KeyStore;
import java.security.cert.X509Certificate;
import java.util.zip.ZipEntry;
import java.util.zip.ZipInputStream;

import javax.net.ssl.TrustManager;
import javax.net.ssl.TrustManagerFactory;
import javax.net.ssl.X509TrustManager;

/** Shared process-lifetime launcher for the embedded Go ledger service. */
final class LedgerRuntime {
    private static final Object START_LOCK = new Object();
    private static boolean backendStarted;
    private static volatile String startupError;

    private LedgerRuntime() {}

    static int port(Context context) {
        return context.getPackageName().endsWith(".qa") ? 18762 : 18761;
    }

    static String origin(Context context) {
        return "http://127.0.0.1:" + port(context);
    }

    static String getStartupError() {
        return startupError;
    }

    static void ensureStarted(Context context) {
        final Context app = context.getApplicationContext();
        final int listenPort = port(app);
        synchronized (START_LOCK) {
            if (backendStarted) return;
            backendStarted = true;
            Thread nativeThread = new Thread(() -> {
                try {
                    File directory = new File(app.getFilesDir(), "ledger");
                    if (!directory.isDirectory() && !directory.mkdirs()) throw new Exception("private directory");
                    prepareAssets(app, directory);
                    prepareTrustStore(directory);
                    MarketNetwork.start(app);
                    startupError = NativeBridge.start(directory.getAbsolutePath(), listenPort);
                } catch (Throwable error) {
                    startupError = "手机账本服务启动失败。请保留应用数据，关闭后重新打开。";
                }
            }, "CYLedger-local-service");
            nativeThread.setDaemon(true);
            nativeThread.start();
        }
    }

    static boolean waitForReady(Context context, long timeoutMs) {
        String baseUrl = origin(context);
        long deadline = System.currentTimeMillis() + timeoutMs;
        while (System.currentTimeMillis() < deadline) {
            if (startupError != null) return false;
            HttpURLConnection connection = null;
            try {
                connection = (HttpURLConnection) new URL(baseUrl + "/healthz.json").openConnection();
                connection.setConnectTimeout(500);
                connection.setReadTimeout(1000);
                if (connection.getResponseCode() == 200) return true;
            } catch (Exception ignored) {
                // Native startup can take a few seconds on cold start.
            } finally {
                if (connection != null) connection.disconnect();
            }
            try {
                Thread.sleep(300);
            } catch (InterruptedException error) {
                Thread.currentThread().interrupt();
                return false;
            }
        }
        return false;
    }

    private static void prepareAssets(Context context, File rootDirectory) throws Exception {
        String revision;
        try (InputStream stream = context.getAssets().open("revision.txt")) {
            revision = new String(readBounded(stream, 1024), StandardCharsets.UTF_8);
        }
        File marker = new File(rootDirectory, "asset-revision");
        if (marker.isFile()) {
            try (InputStream stream = new FileInputStream(marker)) {
                if (revision.equals(new String(readBounded(stream, 1024), StandardCharsets.UTF_8))) return;
            }
        }
        File publicDirectory = new File(rootDirectory, "public");
        if (!publicDirectory.isDirectory() && !publicDirectory.mkdirs()) throw new Exception("assets directory");
        String prefix = publicDirectory.getCanonicalPath() + File.separator;
        try (ZipInputStream zip = new ZipInputStream(context.getAssets().open("frontend.zip"))) {
            ZipEntry entry;
            while ((entry = zip.getNextEntry()) != null) {
                File target = new File(publicDirectory, entry.getName());
                if (!target.getCanonicalPath().startsWith(prefix)) throw new Exception("invalid bundled asset");
                if (entry.isDirectory()) {
                    if (!target.isDirectory() && !target.mkdirs()) throw new Exception("asset directory");
                } else {
                    File parent = target.getParentFile();
                    if (!parent.isDirectory() && !parent.mkdirs()) throw new Exception("asset parent");
                    try (OutputStream output = new FileOutputStream(target)) { copy(zip, output); }
                }
                zip.closeEntry();
            }
        }
        try (InputStream stream = context.getAssets().open("defaults.ini"); OutputStream output = new FileOutputStream(new File(rootDirectory, "defaults.ini"))) {
            copy(stream, output);
        }
        try (InputStream stream = context.getAssets().open("market-network.json"); OutputStream output = new FileOutputStream(new File(rootDirectory, "market-network.json"))) {
            output.write(readBounded(stream, 4096));
        }
        try (OutputStream output = new FileOutputStream(marker)) { output.write(revision.getBytes(StandardCharsets.UTF_8)); }
    }

    private static void prepareTrustStore(File directory) throws Exception {
        TrustManagerFactory factory = TrustManagerFactory.getInstance(TrustManagerFactory.getDefaultAlgorithm());
        factory.init((KeyStore) null);
        try (OutputStream output = new FileOutputStream(new File(directory, "ca-certificates.pem"))) {
            for (TrustManager manager : factory.getTrustManagers()) {
                if (!(manager instanceof X509TrustManager)) continue;
                for (X509Certificate certificate : ((X509TrustManager) manager).getAcceptedIssuers()) {
                    String pem = "-----BEGIN CERTIFICATE-----\n" + Base64.encodeToString(certificate.getEncoded(), Base64.NO_WRAP).replaceAll("(.{64})", "$1\n") + "\n-----END CERTIFICATE-----\n";
                    output.write(pem.getBytes(StandardCharsets.US_ASCII));
                }
            }
        }
    }

    private static void copy(InputStream input, OutputStream output) throws Exception {
        byte[] buffer = new byte[32768];
        int count;
        while ((count = input.read(buffer)) != -1) output.write(buffer, 0, count);
    }

    static byte[] readBounded(InputStream input, int limit) throws Exception {
        ByteArrayOutputStream output = new ByteArrayOutputStream();
        byte[] buffer = new byte[32768];
        int count;
        while ((count = input.read(buffer)) != -1) {
            if (output.size() + count > limit) throw new Exception("file too large");
            output.write(buffer, 0, count);
        }
        return output.toByteArray();
    }
}
