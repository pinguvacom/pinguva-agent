<?php

function pinguva_send_event(array $event): void {
    $url = getenv('PINGUVA_INGEST_URL');
    $token = getenv('PINGUVA_COLLECTOR_TOKEN');
    if (!$url || !$token) {
        return;
    }

    $payload = json_encode(['events' => [$event]], JSON_UNESCAPED_SLASHES);

    $ch = curl_init($url);
    curl_setopt_array($ch, [
        CURLOPT_POST => true,
        CURLOPT_RETURNTRANSFER => true,
        CURLOPT_TIMEOUT_MS => 800,
        CURLOPT_HTTPHEADER => [
            'Authorization: Bearer ' . $token,
            'Content-Type: application/json',
        ],
        CURLOPT_POSTFIELDS => $payload,
    ]);
    curl_exec($ch);
    curl_close($ch);
}
