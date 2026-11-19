document.addEventListener("DOMContentLoaded", () => {
    const logsTab = document.getElementById("logs-tab");
    const refreshBtn = document.getElementById("refreshLogsBtn");
    const logOutput = document.getElementById("logOutput");
    const logLineCount = document.getElementById("logLineCount");
    const logFileSelect = document.getElementById("logFileSelect");
    const downloadBtn = document.getElementById("downloadLogsBtn");

    function updateDownloadHref() {
        downloadBtn.href = `/settings/logs/download?file=${encodeURIComponent(logFileSelect.value)}`;
    }

    function loadLogs() {
        const lines = logLineCount.value;
        const file = logFileSelect.value;
        logOutput.textContent = "Loading...";
        fetch(`/settings/logs?lines=${encodeURIComponent(lines)}&file=${encodeURIComponent(file)}`)
            .then(r => r.json())
            .then(data => {
                logOutput.textContent = data.lines || "(empty)";
                logOutput.scrollTop = logOutput.scrollHeight;
            })
            .catch(() => {
                logOutput.textContent = "Failed to load logs.";
            });
    }

    logsTab.addEventListener("shown.bs.tab", loadLogs);
    refreshBtn.addEventListener("click", loadLogs);
    logLineCount.addEventListener("change", loadLogs);
    logFileSelect.addEventListener("change", () => { updateDownloadHref(); loadLogs(); });
    updateDownloadHref();
});
