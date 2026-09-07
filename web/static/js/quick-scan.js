/**
 * SIMARC Quick Scan — input arsip via kamera/unggah + OCR (Tesseract.js)
 *
 * - Kamera via getUserMedia (annly cam), fallback ke unggah file
 * - OCR berjalan 100% client-side (Tesseract.js load dari CDN) → aman untuk Vercel
 * - Auto-fill field form: nomor_surat, nama_arsip, tanggal, uraian
 *
 * Hanya dijalankan di halaman yang berisi #btnQuickScan.
 */
(function () {
  'use strict';

  var btnOpen = document.getElementById('btnQuickScan');
  if (!btnOpen) return;

  var modalEl = document.getElementById('quickScanModal');
  var video = document.getElementById('scanVideo');
  var canvas = document.getElementById('scanCanvas');
  var resultImg = document.getElementById('scanResult');
  var idle = document.getElementById('scanIdle');
  var captureCtrls = document.getElementById('scanCaptureCtrls');
  var actions = document.getElementById('scanActions');
  var progressWrap = document.getElementById('scanProgressWrap');
  var progressBar = document.getElementById('scanProgressBar');
  var progressText = document.getElementById('scanProgressText');
  var progressPct = document.getElementById('scanProgressPct');
  var ocrResult = document.getElementById('scanOcrResult');
  var ocrText = document.getElementById('scanOcrText');
  var fillWrap = document.getElementById('scanFillWrap');
  var confBadge = document.getElementById('scanConfidence');

  var stream = null;
  var capturedImageDataURL = null;
  var _Tesseract = null;
  var worker = null;

  function $id(id) { return document.getElementById(id); }

  function setStep(step) {
    var s1 = $id('scanStep1'), s2 = $id('scanStep2'), s3 = $id('scanStep3');
    [s1, s2, s3].forEach(function (s) {
      s.style.background = 'var(--surface-3, #e2e8f0)';
      s.style.color = 'var(--ink-3, #334155)';
    });
    if (step === 1) { s1.style.background = 'var(--ink)'; s1.style.color = '#fff'; }
    if (step === 2) { s2.style.background = 'var(--ink)'; s2.style.color = '#fff'; }
    if (step === 3) { s3.style.background = 'var(--ink)'; s3.style.color = '#fff'; }
  }

  function show(el) { if (el) el.style.display = 'block'; }
  function hide(el) { if (el) el.style.display = 'none'; }

  // ── Bootstrap modal helpers (modal is always in DOM) ──
  var bsModal = null;
  function openModal() {
    if (window.bootstrap && window.bootstrap.Modal) {
      bsModal = new window.bootstrap.Modal(modalEl);
      bsModal.show();
    } else {
      modalEl.style.display = 'block';
      modalEl.classList.add('show');
      document.body.style.overflow = 'hidden';
    }
  }
  function closeModal() {
    if (bsModal) { bsModal.hide(); }
    else { modalEl.style.display = 'none'; modalEl.classList.remove('show'); document.body.style.overflow = ''; }
  }

  // ── Camera ──
  async function startCamera() {
    if (!navigator.mediaDevices || !navigator.mediaDevices.getUserMedia) {
      idle.innerHTML = '<i class="bi bi-camera-video-off" style="font-size:2.5rem;"></i><div class="fw-700 mt-2">Kamera tidak didukung. Gunakan tombol Unggah Foto.</div>';
      fallbackToUpload();
      return false;
    }
    hide(idle);
    try {
      // Prefer rear/environment camera for documents
      var facing = 'environment';
      if (window.matchMedia && window.matchMedia('(max-width: 768px)').matches) facing = 'environment';
      stream = await navigator.mediaDevices.getUserMedia({
        video: { facingMode: facing, width: { ideal: 1280 }, height: { ideal: 720 } },
        audio: false
      }).catch(function () {
        // Fallback: any camera
        return navigator.mediaDevices.getUserMedia({ video: true, audio: false });
      });
      video.srcObject = stream;
      show(video);
      hide(resultImg);
      show(captureCtrls);
      hide(actions);
      hide(ocrResult);
      hide(fillWrap);
      setStep(1);
      return true;
    } catch (err) {
      console.warn('Kamera gagal:', err);
      idle.innerHTML = '<i class="bi bi-camera-video-off" style="font-size:2.5rem;"></i><div class="fw-700 mt-2" style="font-size:0.8rem;">Akses kamera ditolak/ tidak tersedia.</div><div class="small opacity-75 mt-1">Unggah foto arsip untuk diproses OCR.</div>';
      fallbackToUpload();
      return false;
    }
  }

  function stopCamera() {
    if (stream) {
      stream.getTracks().forEach(function (t) { t.stop(); });
      stream = null;
    }
    if (video) { video.srcObject = null; hide(video); }
  }

  function fallbackToUpload() {
    hide(video);
    hide(captureCtrls);
    idle.style.display = 'flex';
    idle.innerHTML = '<label for="scanFileUpload" class="btn btn-primary fw-700" style="font-size:0.85rem;cursor:pointer;"><i class="bi bi-images me-2"></i>Pilih / Unggah Foto Dokumen</label>';
    var input = document.getElementById('scanFileUpload');
    if (!input) {
      input = document.createElement('input');
      input.type = 'file';
      input.id = 'scanFileUpload';
      input.accept = 'image/*;capture=camera';
      input.className = 'd-none';
      input.addEventListener('change', onFilePicked);
      modalEl.appendChild(input);
    }
  }

  function onFilePicked(e) {
    var file = e.target.files && e.target.files[0];
    if (!file) return;
    var reader = new FileReader();
    reader.onload = function () {
      capturedImageDataURL = reader.result;
      resultImg.src = capturedImageDataURL;
      show(resultImg);
      hide(video);
      hide(captureCtrls);
      show(actions);
      setStep(1);
    };
    reader.readAsDataURL(file);
  }

  function capture() {
    if (!video || video.readyState < 2) return;
    var vw = video.videoWidth, vh = video.videoHeight;
    if (!vw || !vh) return;
    canvas.width = vw; canvas.height = vh;
    var ctx = canvas.getContext('2d');
    ctx.drawImage(video, 0, 0, vw, vh);
    capturedImageDataURL = canvas.toDataURL('image/jpeg', 0.92);
    resultImg.src = capturedImageDataURL;
    show(resultImg);
    hide(video);
    hide(captureCtrls);
    show(actions);
    setStep(1);
    stopCamera();
  }

  // ── Tesseract loader (lazy, CDN-friendly for Vercel) ──
  function loadTesseract() {
    return new Promise(function (resolve, reject) {
      if (window.Tesseract) { _Tesseract = window.Tesseract; resolve(); return; }
      var s = document.createElement('script');
      s.src = 'https://cdn.jsdelivr.net/npm/tesseract.js@4.1.3/dist/tesseract.min.js';
      s.crossOrigin = 'anonymous';
      s.onload = function () {
        _Tesseract = window.Tesseract;
        resolve();
      };
      s.onerror = function () {
        reject(new Error('Gagal memuat Tesseract.js dari CDN. Periksa koneksi internet.'));
      };
      document.head.appendChild(s);
    });
  }

  async function runOCR() {
    if (!capturedImageDataURL) return;
    setStep(2);
    hide(actions);
    hide(ocrResult);
    hide(fillWrap);
    show(progressWrap);
    progressBar.style.width = '0%';
    progressPct.textContent = '0%';
    progressText.textContent = 'Menyiapkan mesin OCR...';

    try {
      await loadTesseract();
      progressText.textContent = 'Menginisialisasi worker...';
      worker = _Tesseract.createWorker({
        logger: function (m) {
          if (m && m.status) {
            progressText.textContent = m.status;
            if (typeof m.progress === 'number') {
              var pct = Math.round(m.progress * 100);
              progressBar.style.width = pct + '%';
              progressPct.textContent = pct + '%';
            }
          }
        }
      });
      await worker.load();
      await worker.loadLanguage('ind');   // Bahasa Indonesia
      await worker.initialize('ind');
      var result = await worker.recognize(capturedImageDataURL);
      await worker.terminate();

      progressText.textContent = 'Selesai';
      progressBar.style.width = '100%';
      progressPct.textContent = '100%';

      var text = (result && result.data && result.data.text) || '';
      var confidence = result && result.data && typeof result.data.confidence === 'number'
        ? Math.round(result.data.confidence) : 0;

      setTimeout(function () {
        hide(progressWrap);
        show(ocrResult);
        ocrText.value = text.trim();
        if (confBadge) confBadge.textContent = 'Akurasi ~' + confidence + '%';
        if (text.trim().length > 0) {
          show(fillWrap);
          setStep(3);
        } else {
          hide(fillWrap);
          setStep(2);
          if (confBadge) confBadge.textContent = 'Tidak ada teks terdeteksi';
        }
      }, 250);
    } catch (err) {
      progressText.textContent = 'OCR gagal: ' + err.message;
      progressBar.style.width = '100%';
      setTimeout(function () {
        hide(progressWrap);
        show(actions);
        setStep(2);
      }, 300);
    }
  }

  // ── Parse hasil OCR → isi form ──
  function extractFields(text) {
    var out = { nomor: '', nama: '', tanggal: '', uraian: '' };
    if (!text) return out;

    // Normalisasi (hapus baris kosong & trim)
    var lines = text.split('\n').map(function (l) { return l.trim(); }).filter(Boolean);

    // 1. Nomor surat/arsip: bentuk "000/SPJ/2024", "SPM-2024-001", "Nomor: 123", dll
    var nomorPatterns = [
      /(?:Nomor|No\.?|No)\s*[:\-]?\s*([A-Z0-9][A-Z0-9\/\-\s\.]{4,})/i,
      /(?:nomor\s*(?:surat|arsip|sptj|spm)?\s*:\s*)([A-Z0-9][A-Z0-9\/\-\s\.]{4,})/i,
      /\b(?:SPM|SPJ|SPP|SK|SP)\s*[-]?\s*\d{1,5}\s*\/\s*\d{1,4}/i,
      /\b\d{1,4}\s*\/\s*(?:Ao|BA|BUP|BJ|DAK|KPA|PA|PPK|S|SJG|SPM|SPJ|SK|SKK|SPP|SS|ST)\s*\/\s*\d{4}\b/i
    ];
    for (var i = 0; i < nomorPatterns.length; i++) {
      var m = text.match(nomorPatterns[i]);
      if (m && m[1] || (m && m[0])) {
        out.nomor = (m[1] || m[0]).replace(/\s+/g, ' ').trim().replace(/[;:,]$/, '');
        if (out.nomor.length > 2) break;
      }
    }

    // 2. Nama arsip — baris pertama yang terlihat seperti judul (panjang 6-120, tidak nomor/tanggal/alamat)
    for (var j = 0; j < lines.length; j++) {
      var line = lines[j];
      var looksLikeNo = /^[\s×Xx*#•·\-–—.]+$/.test(line) || /^\d+[\.\/]\s*$/.test(line) || /^-+$/.test(line);
      var len = line.length;
      if (!looksLikeNo && line && len >= 6 && len <= 140 && !/^\d{1,3}$/.test(line)) {
        out.nama = line.replace(/\s{2,}/g, ' ');
        break;
      }
    }

    // 3. Tanggal
    var dateMatchers = [
      /(?:\d{1,2}\s+(?:Januari|Februari|Maret|April|Mei|Juni|Juli|Agustus|September|Oktober|November|Desember)\s+\d{4})/i,
      /\b\d{1,2}\/\d{1,2}\/\d{2,4}\b/,
      /\b\d{4}-\d{1,2}-\d{1,2}\b/
    ];
    for (var d = 0; d < dateMatchers.length; d++) {
      var dm = text.match(dateMatchers[d]);
      if (dm) { out.tanggal = dm[0]; break; }
    }

    // 4. Uraian — kumpulkan beberapa baris pertama selain nomor/nama yang sudah dipakai
    var uraiLines = [];
    for (var k = 1; k < Math.min(lines.length, 6); k++) {
      var t = lines[k];
      if (!t) continue;
      if (out.nama && t === out.nama) continue;
      if (t.length < 3) continue;
      uraiLines.push(t);
      if (uraiLines.length >= 3) break;
    }
    out.uraian = uraiLines.join(' ');
    return out;
  }

  function parseDateToInput(val) {
    var m = val.match(/(\d{1,2})\s+([A-Za-z]+)\s+(\d{4})/);
    if (m) {
      var months = { 'januari':1,'februari':2,'maret':3,'april':4,'mei':5,'juni':6,'juli':7,'agustus':8,'september':9,'oktober':10,'november':11,'desember':12 };
      var mm = months[m[2].toLowerCase()];
      if (mm) return m[3] + '-' + String(mm).padStart(2,'0') + '-' + String(m[1]).padStart(2,'0');
    }
    m = val.match(/(\d{1,2})\/(\d{1,2})\/(\d{2,4})/);
    if (m) {
      var y = m[3].length === 2 ? '20' + m[3] : m[3];
      return y + '-' + String(m[2]).padStart(2,'0') + '-' + String(m[1]).padStart(2,'0');
    }
    m = val.match(/(\d{4})-(\d{1,2})-(\d{1,2})/);
    if (m) return m[1] + '-' + String(m[2]).padStart(2,'0') + '-' + String(m[3]).padStart(2,'0');
    return '';
  }

  function fillForm() {
    var text = ocrText.value || '';
    var fields = extractFields(text);

    // Nomor
    if (fields.nomor && !document.querySelector('input[name="nomor_arsip"]').value) {
      document.querySelector('input[name="nomor_arsip"]').value = fields.nomor;
    }
    // Nama
    if (fields.nama && !document.querySelector('input[name="nama_arsip"]').value) {
      document.querySelector('input[name="nama_arsip"]').value = fields.nama;
    }
    // Tanggal
    if (fields.tanggal) {
      var iso = parseDateToInput(fields.tanggal);
      var tglInput = document.querySelector('input[name="tanggal_dibuat"]');
      if (iso && tglInput) {
        tglInput.value = iso;
        tglInput.dispatchEvent(new Event('change', { bubbles: true }));
      }
    }
    // Uraian
    if (fields.uraian && !document.querySelector('textarea[name="uraian"]').value) {
      document.querySelector('textarea[name="uraian"]').value = fields.uraian;
    }

    // Flash feedback
    if (typeof SimarcModal !== 'undefined' && SimarcModal.toast) {
      SimarcModal.toast('Form terisi dari hasil scan!', 'success');
    }
    closeModal();
    resetScan();
  }

  function resetScan() {
    hide(resultImg);
    hide(actions);
    hide(ocrResult);
    hide(fillWrap);
    hide(progressWrap);
    show(idle);
    idle.innerHTML = '<i class="bi bi-camera-video-fill" style="font-size:3rem;"></i><div class="fw-700 mt-2" style="font-size:0.85rem;">Menyiapkan kamera...</div>';
    setStep(1);
    capturedImageDataURL = null;
    ocrText.value = '';
  }

  // ── Event wiring ──
  btnOpen.addEventListener('click', function () {
    openModal();
    resetScan();
    setTimeout(function () { startCamera(); }, 300);
  });

  var btnCapture = $id('btnCapture');
  if (btnCapture) btnCapture.addEventListener('click', capture);

  var btnRetake = $id('btnRetake');
  if (btnRetake) btnRetake.addEventListener('click', function () {
    hide(resultImg);
    hide(actions);
    hide(ocrResult);
    hide(fillWrap);
    startCamera();
  });

  var btnOcr = $id('btnStartOcr');
  if (btnOcr) btnOcr.addEventListener('click', runOCR);

  var btnFill = $id('btnFillForm');
  if (btnFill) btnFill.addEventListener('click', fillForm);

  var btnClose = $id('btnCloseScan');
  if (btnClose) btnClose.addEventListener('click', function () { stopCamera(); });

  modalEl.addEventListener('hidden.bs.modal', function () { stopCamera(); });

  // Re-open camera mount if open programmatically
  var fileInput = $id('scanFileUpload');
  if (fileInput) fileInput.addEventListener('change', onFilePicked);
})();