// FlowCanvas - Interactive Node Graph Workflow Canvas (n8n & ComfyUI style)
(function () {
  'use strict';

  const escapeHTML = value => String(value ?? '').replace(/[&<>"']/g, char => ({
    '&': '&amp;', '<': '&lt;', '>': '&gt;', '"': '&quot;', "'": '&#39;'
  })[char]);

  const safeMediaURL = value => {
    const raw = String(value || '').trim();
    if (/^data:image\/(?:png|jpe?g|gif|webp);base64,[a-z0-9+/=]+$/i.test(raw)) return raw;
    try {
      const parsed = new URL(raw, window.location.origin);
      return parsed.protocol === 'http:' || parsed.protocol === 'https:' ? parsed.href : '';
    } catch (_) {
      return '';
    }
  };

  const workflowMediaURL = value => {
    const raw = String(value || '').trim();
    if (!raw) return '';
    if (/^data:image\//i.test(raw) || /^https?:\/\//i.test(raw) || /^\/(?:media|static|studio\/uploads|uploads)\//i.test(raw)) {
      return safeMediaURL(raw);
    }
    const filename = raw.split(/[\\/]/).pop();
    return filename ? safeMediaURL(`/media/${encodeURIComponent(filename)}`) : '';
  };

  const flowGraphMayUseProviderCredits = graph => (graph?.nodes || []).some(node => {
    const type = String(node?.type || '').toLowerCase();
    const defaultOutputType = type === 'custom_web_ai' ? 'video' : type === 'custom_universal_ai' ? 'auto' : 'text';
    const outputType = String(node?.params?.output_type || defaultOutputType).toLowerCase();
    return type === 'muse_ai_video' || type === 'muse_ai_neural_render' || type === 'google_flow_video' ||
      (['custom_web_ai', 'custom_ai_prompt', 'custom_universal_ai'].includes(type) && outputType === 'video') ||
      type === 'workflow_map' || type === 'workflow_call' || type.startsWith('workflow_call_');
  });

  const FLOW_CLIPBOARD_PREFIX = 'MIUNOVA_FLOW_CLIPBOARD_V1:';
  const FLOW_CLIPBOARD_STORAGE_KEY = 'miunova.workflow.clipboard.v1';

  class FlowCanvas {
    constructor() {
      this.container = document.getElementById('flow-canvas-container');
      this.viewport = document.getElementById('flow-canvas-viewport');
      this.svgLayer = document.getElementById('flow-wires-svg');
      this.nodesLayer = document.getElementById('flow-nodes-layer');
      this.inspector = document.getElementById('flow-inspector');
      this.nodeCountBadge = document.getElementById('wf-node-count-badge');
      this.statusPill = document.getElementById('wf-status-pill');

      this.graph = window.__INITIAL_GRAPH__ || { id: 'custom_flow', name: 'Luồng tùy chỉnh', nodes: [], edges: [] };
      if (!this.graph.nodes) this.graph.nodes = [];
      if (!this.graph.edges) this.graph.edges = [];

      this.definitions = {};
      (window.__NODE_DEFINITIONS__ || []).forEach(d => {
        this.definitions[d.type] = d;
      });

      this.scale = 1.0;
      this.panX = 40;
      this.panY = 40;
      this.isPanning = false;
      this.panStartX = 0;
      this.panStartY = 0;

      this.selectedNodeId = null;
      this.activeConnecting = null; // { nodeId, portId, isOutput, startX, startY }
      this.tempWire = null;
      this.pendingConnect = null; // { fromNode, fromPort } for clicking '+' handle

      this.currentRunId = null;
      this.lastWorkflowInputs = {};
      this.pollInterval = null;
      this.pendingUploads = new Map();
      this.uploadErrors = new Map();
      this.mediaUploadSequence = new Map();
      this.pendingRunInputUploads = 0;
      this.savedGraphs = window.__SAVED_GRAPHS__ || [];
      this.workflowContractLoading = new Set();
      this.workflowContractRequests = new Map();
      this.graphPersisted = Boolean(this.graph.created_at || this.graph.is_preset);
      this.savedGraphFingerprint = this.graphFingerprint();

      this.initEvents();
      this.setupResizeObserver();
      this.applyTransform();
      this.render();
      this.bindGraphMetadata();
      this.bindToolbar();
      const runPanelClose = document.getElementById('btn-close-run-panel');
      if (runPanelClose) runPanelClose.addEventListener('click', () => {
        const panel = document.getElementById('flow-run-panel');
        if (panel) panel.hidden = true;
      });
      this.bindInspector();
      this.bindModal();
      this.bindGraphsModal();
      this.updateZoomDisplay();
      if (this.graph.nodes.length) requestAnimationFrame(() => this.fitCanvas());
    }

    setupResizeObserver() {
      if (window.ResizeObserver) {
        this.resizeObserver = new ResizeObserver(() => {
          if (this.wiresRaf) cancelAnimationFrame(this.wiresRaf);
          this.wiresRaf = requestAnimationFrame(() => {
            this.renderWires();
          });
        });
        if (this.nodesLayer) {
          this.resizeObserver.observe(this.nodesLayer);
        }
      }
    }

    initEvents() {
      // Pan canvas by dragging background
      this.container.addEventListener('mousedown', (e) => {
        if (e.target === this.container || e.target === this.viewport || e.target === this.svgLayer) {
          this.isPanning = true;
          this.panStartX = e.clientX - this.panX;
          this.panStartY = e.clientY - this.panY;
          this.container.style.cursor = 'grabbing';
          this.deselectAll();
        }
      });

      window.addEventListener('mousemove', (e) => {
        if (this.isPanning) {
          this.panX = e.clientX - this.panStartX;
          this.panY = e.clientY - this.panStartY;
          this.applyTransform();
        } else if (this.activeConnecting) {
          this.updateTempWire(e.clientX, e.clientY);
        }
      });

      window.addEventListener('mouseup', () => {
        if (this.isPanning) {
          this.isPanning = false;
          this.container.style.cursor = 'default';
        }
        if (this.activeConnecting) {
          this.cancelConnecting();
        }
      });

      // Zoom via mouse wheel toward mouse point
      this.container.addEventListener('wheel', (e) => {
        e.preventDefault();
        const zoomFactor = 1.1;
        const rect = this.container.getBoundingClientRect();
        const mouseX = e.clientX - rect.left;
        const mouseY = e.clientY - rect.top;

        const prevScale = this.scale;
        if (e.deltaY < 0) {
          this.scale = Math.min(2.5, this.scale * zoomFactor);
        } else {
          this.scale = Math.max(0.35, this.scale / zoomFactor);
        }

        this.panX = mouseX - (mouseX - this.panX) * (this.scale / prevScale);
        this.panY = mouseY - (mouseY - this.panY) * (this.scale / prevScale);

        this.applyTransform();
        this.updateZoomDisplay();
      }, { passive: false });

      window.addEventListener('resize', () => {
        this.renderWires();
      });

      // Keyboard shortcuts
      window.addEventListener('keydown', (e) => {
        const target = e.target instanceof Element ? e.target : null;
        const isEditingText = Boolean(target?.closest('input, textarea, select, [contenteditable="true"], [role="textbox"]'));
        if ((e.metaKey || e.ctrlKey) && !e.altKey && !isEditingText) {
          if (e.key.toLowerCase() === 'c') {
            e.preventDefault();
            void this.copyCanvasContent();
            return;
          }
          if (e.key.toLowerCase() === 'v') {
            e.preventDefault();
            void this.pasteCanvasContent();
            return;
          }
        }
        if ((e.key === 'Delete' || e.key === 'Backspace') && this.selectedNodeId) {
          const activeTag = document.activeElement ? document.activeElement.tagName.toLowerCase() : '';
          if (activeTag === 'input' || activeTag === 'textarea' || activeTag === 'select' || isEditingText) return;
          this.deleteNode(this.selectedNodeId);
        } else if (e.key === '+' && !['input', 'textarea'].includes(document.activeElement?.tagName.toLowerCase())) {
          this.openAddNodeModal();
        } else if (e.key === 'Escape') {
          this.closeAddNodeModal();
          this.cancelConnecting();
        }
      });
    }

    applyTransform() {
      this.viewport.style.transform = `translate(${this.panX}px, ${this.panY}px) scale(${this.scale})`;
    }

    updateZoomDisplay() {
      const zoomText = document.getElementById('zoom-level');
      if (zoomText) {
        zoomText.innerText = `${Math.round(this.scale * 100)}%`;
      }
    }

    fitCanvas() {
      if (!this.graph.nodes?.length) return;
      const width = this.container.clientWidth;
      const height = this.container.clientHeight;
      if (!width || !height) return;
      const bounds = this.graph.nodes.reduce((rect, node) => {
        const element = document.getElementById(`node-${node.id}`);
        const x = Number(node.x) || 0;
        const y = Number(node.y) || 0;
        const nodeWidth = element?.offsetWidth || 280;
        const nodeHeight = element?.offsetHeight || 180;
        rect.left = Math.min(rect.left, x);
        rect.top = Math.min(rect.top, y);
        rect.right = Math.max(rect.right, x + nodeWidth);
        rect.bottom = Math.max(rect.bottom, y + nodeHeight);
        return rect;
      }, { left: Infinity, top: Infinity, right: -Infinity, bottom: -Infinity });
      const graphWidth = Math.max(1, bounds.right - bounds.left);
      const graphHeight = Math.max(1, bounds.bottom - bounds.top);
      this.scale = Math.min((width - 64) / graphWidth, (height - 112) / graphHeight, 1.25);
      this.scale = Math.max(0.2, this.scale);
      this.panX = (width - graphWidth * this.scale) / 2 - bounds.left * this.scale;
      this.panY = (height - graphHeight * this.scale) / 2 - bounds.top * this.scale;
      this.applyTransform();
      this.updateZoomDisplay();
      requestAnimationFrame(() => this.renderWires());
    }

    render() {
      this.nodesLayer.innerHTML = '';
      if (this.graph.nodes && this.graph.nodes.length > 0) {
        this.graph.nodes.forEach(node => {
          this.renderNode(node);
        });
      }
      this.renderWires();
      if (this.nodeCountBadge) {
        this.nodeCountBadge.innerText = `${(this.graph.nodes || []).length} nodes`;
      }
    }

    renderNode(node) {
      const isUnsupported = !this.definitions[node.type];
      const def = this.definitions[node.type] || {
        type: node.type,
        title: node.title || node.type,
        category: node.category || 'process',
        inputs: [],
        outputs: []
      };

      const cat = node.category || def.category || 'process';

      // Đảm bảo không rò rỉ preview sang các node xử lý/AI chưa chạy
      if (this.definitions[node.type] && !this.isInputNodeType(node.type)) {
        if (node.params && node.params.preview_url && (!node.state || node.state.status !== 'completed')) {
          delete node.params.preview_url;
        }
      }

      const nodeEl = document.createElement('div');
      nodeEl.className = `wf-node cat-${cat}`;
      if (isUnsupported) nodeEl.classList.add('wf-node-unsupported');
      nodeEl.id = `node-${node.id}`;
      nodeEl.dataset.nodeId = node.id;
      nodeEl.style.left = `${node.x || 100}px`;
      nodeEl.style.top = `${node.y || 100}px`;

      if (this.selectedNodeId === node.id) {
        nodeEl.classList.add('selected');
      }

      // 1. Node Header Main (n8n Card layout)
      const header = document.createElement('div');
      header.className = 'wf-node-main';
      const nodeTitle = escapeHTML(node.title || def.title || node.type);
      const definitionTitle = escapeHTML(isUnsupported ? 'Node cũ · chưa hỗ trợ thực thi' : (def.title || node.type));
      const safeNodeId = escapeHTML(node.id);
      const safeCategory = escapeHTML(cat);
      const model = node.params && (node.params.model || node.params.provider);
      const shortModel = this.formatModelShortName(model);
      const modelTag = model
        ? `<span class="wf-node-model-pill" id="node-model-${safeNodeId}">${escapeHTML(shortModel)}</span>`
        : `<span class="wf-node-model-pill" id="node-model-${safeNodeId}" style="display:none;"></span>`;
      header.innerHTML = `
        <div class="wf-node-icon cat-${safeCategory}">
          <span>${this.getCategoryIcon(cat)}</span>
        </div>
        <div class="wf-node-info">
          <div class="wf-node-title" id="node-title-${safeNodeId}" title="${nodeTitle}">${nodeTitle}</div>
          <div class="wf-node-subtitle" title="${definitionTitle}">
            ${modelTag}
            <span>${definitionTitle}</span>
            ${isUnsupported ? '<span class="wf-node-legacy-badge">Node cũ</span>' : ''}
          </div>
        </div>
        <div class="wf-node-actions">
          <button type="button" class="wf-node-edit-btn" title="Mở chi tiết và cấu hình node">⚙️</button>
          <button type="button" class="wf-node-del-btn" title="Xóa node">&times;</button>
        </div>
      `;

      // Open inline node detail
      const editBtn = header.querySelector('.wf-node-edit-btn');
      if (editBtn) {
        editBtn.addEventListener('click', (e) => {
          e.stopPropagation();
          this.selectNode(node.id, true);
        });
      }

      // Delete Node
      header.querySelector('.wf-node-del-btn').addEventListener('click', (e) => {
        e.stopPropagation();
        this.deleteNode(node.id);
      });

      // Drag Node (Entire node card is smoothly draggable with real-time wire auto-stretch)
      let startClientX = 0, startClientY = 0;
      let startNodeX = 0, startNodeY = 0;
      let isDraggingNode = false;
      let hasDragged = false;
      let rafId = null;

      nodeEl.addEventListener('mousedown', (e) => {
        if (e.button !== 0) return;

        // Ignore interactive targets (port dots, add button, delete button, inputs, selects)
        if (e.target.closest('.wf-port-dot') ||
            e.target.closest('.wf-port-add-btn') ||
            e.target.closest('.wf-node-del-btn') ||
            e.target.closest('button') ||
            e.target.closest('input') ||
            e.target.closest('textarea') ||
            e.target.closest('select') ||
            e.target.closest('video') ||
            e.target.closest('a')) {
          return;
        }

        e.stopPropagation();
        e.preventDefault(); // Prevent browser text selection ghost drag

        // Visually highlight node without opening popup detail
        this.selectedNodeId = node.id;
        this.nodesLayer.querySelectorAll('.wf-node').forEach(el => {
          el.classList.toggle('selected', el.id === `node-${node.id}`);
        });

        isDraggingNode = true;
        hasDragged = false;
        startClientX = e.clientX;
        startClientY = e.clientY;
        startNodeX = typeof node.x === 'number' ? node.x : (parseFloat(nodeEl.style.left) || 100);
        startNodeY = typeof node.y === 'number' ? node.y : (parseFloat(nodeEl.style.top) || 100);

        const onMouseMove = (ev) => {
          if (!isDraggingNode) return;
          const dist = Math.hypot(ev.clientX - startClientX, ev.clientY - startClientY);
          if (dist > 8) {
            hasDragged = true;
            nodeEl.classList.add('is-dragging');
            document.body.style.userSelect = 'none';

            const dx = (ev.clientX - startClientX) / this.scale;
            const dy = (ev.clientY - startClientY) / this.scale;

            node.x = Math.round(startNodeX + dx);
            node.y = Math.round(startNodeY + dy);

            nodeEl.style.left = `${node.x}px`;
            nodeEl.style.top = `${node.y}px`;

            this.renderWires();
          }
        };

        const onMouseUp = (ev) => {
          if (isDraggingNode) {
            isDraggingNode = false;
            nodeEl.classList.remove('is-dragging');
            document.body.style.userSelect = '';
          }
          if (rafId) {
            cancelAnimationFrame(rafId);
            rafId = null;
          }
          window.removeEventListener('mousemove', onMouseMove);
          window.removeEventListener('mouseup', onMouseUp);

          const totalDist = Math.hypot((ev.clientX || startClientX) - startClientX, (ev.clientY || startClientY) - startClientY);
          if (totalDist > 8) {
            hasDragged = true;
            this.renderWires();
          } else {
            hasDragged = false;
          }
        };

        window.addEventListener('mousemove', onMouseMove);
        window.addEventListener('mouseup', onMouseUp);
      });

      nodeEl.appendChild(header);

      // 2. Ports Row (Inputs on Left, Outputs on Right with + Handle)
      const portsRow = document.createElement('div');
      portsRow.className = 'wf-ports-row';

      const inputsCol = document.createElement('div');
      inputsCol.className = 'wf-ports-inputs';
      const outputsCol = document.createElement('div');
      outputsCol.className = 'wf-ports-outputs';

      const inputs = this.getNodePorts(node, false);
      inputs.forEach(port => {
        inputsCol.appendChild(this.createPortElement(node.id, port, false));
      });

      const outputs = this.getNodePorts(node, true);
      outputs.forEach(port => {
        outputsCol.appendChild(this.createPortElement(node.id, port, true));
      });

      portsRow.appendChild(inputsCol);
      portsRow.appendChild(outputsCol);
      nodeEl.appendChild(portsRow);

      // 3. Node Live Preview (ComfyUI Visual Feedback: Cutout, Composite, Video, etc.)
      const previewEl = this.createNodePreviewElement(node);
      if (previewEl) {
        nodeEl.appendChild(previewEl);
      }

      // 4. Node Footer & Status Indicator
      const footer = document.createElement('div');
      footer.className = 'wf-node-footer';
      footer.id = `node-footer-${node.id}`;
      footer.innerHTML = this.renderNodeStatus(node.state || { status: 'idle' }, node.type);
      nodeEl.appendChild(footer);

      // Click anywhere on node to select and open modal detail (ONLY if user didn't drag/move)
      nodeEl.addEventListener('click', (e) => {
        if (e.target.closest('button') || e.target.closest('input') || e.target.closest('select') || e.target.closest('textarea')) return;
        e.stopPropagation();
        if (hasDragged) {
          hasDragged = false;
          return;
        }
        this.selectNode(node.id, true);
      });

      nodeEl.addEventListener('dblclick', (e) => {
        if (e.target.closest('button') || e.target.closest('input') || e.target.closest('select') || e.target.closest('textarea')) return;
        e.stopPropagation();
        this.selectNode(node.id, true);
      });

      this.nodesLayer.appendChild(nodeEl);
      if (this.resizeObserver) {
        this.resizeObserver.observe(nodeEl);
      }
    }

    createPortElement(nodeId, port, isOutput) {
      const portEl = document.createElement('div');
      portEl.className = `wf-port ${isOutput ? 'wf-port-output' : 'wf-port-input'} port-type-${port.type || 'any'} ${port.legacy ? 'wf-port-legacy' : ''}`;
      portEl.dataset.nodeId = nodeId;
      portEl.dataset.portId = port.id;
      portEl.dataset.type = port.type || 'any';
      portEl.dataset.isOutput = isOutput ? '1' : '0';

      const dot = document.createElement('span');
      dot.className = 'wf-port-dot';
      dot.title = `${port.name} (${port.type})`;

      const label = document.createElement('span');
      label.className = 'wf-port-label';
      label.innerText = port.name;

      if (isOutput) {
        dot.innerHTML = port.legacy ? '<span class="wf-port-legacy-mark">!</span>' : '<span class="wf-port-plus">+</span>';
        dot.title = port.legacy ? `Cổng lưu từ phiên bản cũ (${port.name}); node chưa hỗ trợ thực thi.` : `Kéo để nối dây hoặc bấm (+) để thêm bước tiếp theo (${port.name})`;
        portEl.appendChild(label);
        portEl.appendChild(dot);

        // Click / Drag handle logic for output port (+)
        let isDrag = false;
        let startClientX = 0, startClientY = 0;

        if (!port.legacy) dot.addEventListener('mousedown', (e) => {
          if (e.button !== 0) return;
          e.stopPropagation();

          isDrag = false;
          startClientX = e.clientX;
          startClientY = e.clientY;

          const p = this.getPortCoordinates(dot);
          const startX = p.x;
          const startY = p.y;

          this.activeConnecting = {
            nodeId,
            portId: port.id,
            portType: port.type,
            startX,
            startY
          };

          const onMouseMove = (ev) => {
            const dist = Math.hypot(ev.clientX - startClientX, ev.clientY - startClientY);
            if (dist > 3 && !isDrag) {
              isDrag = true;
              this.createTempWire(startX, startY);
            }
          };

          const onMouseUp = (ev) => {
            window.removeEventListener('mousemove', onMouseMove);
            window.removeEventListener('mouseup', onMouseUp);

            if (!isDrag) {
              // Clicked (+) without dragging: open Add Node Modal and auto-connect
              this.cancelConnecting();
              this.pendingConnect = {
                fromNode: nodeId,
                fromPort: port.id,
                portType: port.type
              };
              this.openAddNodeModal();
            }
          };

          window.addEventListener('mousemove', onMouseMove);
          window.addEventListener('mouseup', onMouseUp);
        });
      } else {
        dot.title = port.legacy ? `Cổng lưu từ phiên bản cũ (${port.name}); node chưa hỗ trợ thực thi.` : `${port.name} (${port.type})`;
        portEl.appendChild(dot);
        portEl.appendChild(label);

        // Drop wire connection on input dot
        if (!port.legacy) dot.addEventListener('mouseup', (e) => {
          e.stopPropagation();
          if (this.activeConnecting) {
            const fromNode = this.activeConnecting.nodeId;
            const fromPort = this.activeConnecting.portId;
            const toNode = nodeId;
            const toPort = port.id;

            if (fromNode !== toNode) {
              this.addEdge(fromNode, fromPort, toNode, toPort);
            }
            this.cancelConnecting();
          }
        });
      }

      return portEl;
    }

    createNodePreviewElement(node) {
      const config = this.getNodePreviewConfig(node);
      if (!config) return null;
      const mediaURL = config.url ? workflowMediaURL(config.url) : '';
      if (config.url && !mediaURL) return null;

      // Không render nếu không có url, không cho phép upload và không phải dedicated preview
      if (!config.url && !config.canUpload && !config.isDedicatedPreview) return null;

      const previewContainer = document.createElement('div');
      previewContainer.className = `wf-node-preview ${config.isCutout ? 'preview-cutout' : ''} ${config.isDedicatedPreview ? 'preview-dedicated' : ''}`;
      previewContainer.id = `node-preview-${node.id}`;

      // Badge
      const badge = document.createElement('div');
      badge.className = 'wf-preview-badge';
      badge.innerHTML = config.badge;
      previewContainer.appendChild(badge);

      // Media element
      if (config.url) {
        if (config.type === 'video') {
          const video = document.createElement('video');
          video.src = mediaURL;
          video.muted = true;
          video.loop = true;
          video.playsInline = true;
          video.preload = 'metadata';
          video.onloadedmetadata = () => { this.renderWires(); };
          video.onerror = () => { video.style.display = 'none'; };
          // Auto play on hover
          previewContainer.addEventListener('mouseenter', () => {
            video.play().catch(() => {});
          });
          previewContainer.addEventListener('mouseleave', () => {
            video.pause();
          });
          previewContainer.appendChild(video);
        } else if (config.type === 'image' || config.type === 'cutout') {
          const img = document.createElement('img');
          img.src = mediaURL;
          img.alt = '';
          img.loading = 'lazy';
          img.onload = () => { this.renderWires(); };
          img.onerror = () => { img.style.display = 'none'; };
          previewContainer.appendChild(img);
        }
      } else {
        const placeholder = document.createElement('div');
        placeholder.className = 'wf-preview-empty-placeholder';
        placeholder.style.cssText = 'padding:16px 12px;text-align:center;font-size:12px;color:var(--text-muted, #94a3b8);background:rgba(0,0,0,0.18);border-radius:6px;width:100%;';
        if (config.isDedicatedPreview) {
          placeholder.innerHTML = '<span>👁️ <b>Chờ kết nối</b><br>Nối dây từ cổng ra của bất kỳ node nào vào đây để xem kết quả trực tiếp</span>';
        } else {
          placeholder.innerHTML = '<span>Chưa có mẫu. Bấm <b>Tải lên</b> hoặc <b>Dán link</b></span>';
        }
        previewContainer.appendChild(placeholder);
      }

      // Quick actions toolbar
      const actions = document.createElement('div');
      actions.className = 'wf-preview-actions';

      // 1. Zoom / Lightbox button
      if (config.url) {
        const zoomBtn = document.createElement('button');
        zoomBtn.type = 'button';
        zoomBtn.className = 'wf-preview-btn';
        zoomBtn.title = 'Phóng to xem chi tiết';
        zoomBtn.innerHTML = '🔍 Xem';
        zoomBtn.addEventListener('click', (e) => {
          e.stopPropagation();
          this.openLightbox(node.title || 'Preview', mediaURL, config.type);
        });
        actions.appendChild(zoomBtn);

        const dlBtn = document.createElement('button');
        dlBtn.type = 'button';
        dlBtn.className = 'wf-preview-btn';
        dlBtn.title = 'Tải file kết quả về máy';
        dlBtn.innerHTML = '💾 Tải';
        dlBtn.addEventListener('click', (e) => {
          e.stopPropagation();
          const fn = this.getFilename(config.url) || 'result_preview';
          const a = document.createElement('a');
          a.href = mediaURL;
          a.download = fn;
          document.body.appendChild(a);
          a.click();
          document.body.removeChild(a);
        });
        actions.appendChild(dlBtn);
      }

      // 2. Paste Link button (TikTok, Reels, IG, Web Video/Image)
      if (config.canFetchUrl || config.canUpload) {
        const linkBtn = document.createElement('button');
        linkBtn.type = 'button';
        linkBtn.className = 'wf-preview-btn';
        linkBtn.title = 'Nhập link video TikTok, Reels, IG hoặc ảnh/video từ web';
        linkBtn.innerHTML = '🔗 Dán link';
        linkBtn.addEventListener('click', async (e) => {
          e.stopPropagation();
          const currentUrl = node.params?.url || node.params?.video_url || '';
          const inputUrl = await window.showAppPrompt('Dán đường link (TikTok, Instagram Reels, IG, YouTube Shorts hoặc link web):', currentUrl, { label: 'Đường dẫn media', placeholder: 'https://…', confirmText: 'Tải media' });
          if (inputUrl && inputUrl.trim()) {
            await this.fetchMediaFromUrl(node, inputUrl.trim(), config.type === 'cutout' ? 'image' : config.type);
          }
        });
        actions.appendChild(linkBtn);
      }

      // 3. Upload button (if node supports uploading input asset)
      if (config.canUpload) {
        const uploadBtn = document.createElement('button');
        uploadBtn.type = 'button';
        uploadBtn.className = 'wf-preview-btn';
        uploadBtn.title = 'Tải ảnh/video mới từ máy tính lên node này';
        uploadBtn.innerHTML = '📤 Tải lên';

        const fileInput = document.createElement('input');
        fileInput.type = 'file';
        fileInput.accept = (node.type === 'product_extractor' || node.type === 'background_remover')
          ? 'image/*,video/*,video/mp4'
          : (config.type === 'video' ? 'video/mp4,video/*' : 'image/*');
        fileInput.style.display = 'none';

        fileInput.addEventListener('change', async (ev) => {
          const file = ev.target.files && ev.target.files[0];
          if (!file) return;
          const uploadSequence = (this.mediaUploadSequence.get(node.id) || 0) + 1;
          this.mediaUploadSequence.set(node.id, uploadSequence);
          this.uploadErrors.delete(node.id);

          // Preview cục bộ chỉ dùng để hiển thị; không đưa data URL vào graph params.
          const reader = new FileReader();
          reader.onload = (re) => {
            if (this.mediaUploadSequence.get(node.id) !== uploadSequence) return;
            const dataUrl = re.target.result;
            this.updateNodePreview(node.id, dataUrl);
          };
          reader.readAsDataURL(file);

          const uploadTask = (async () => {
            const formData = new FormData();
            formData.append('file', file);
            const resp = await fetch('/api/flow/upload', {
              method: 'POST',
              body: formData
            });
            const res = await resp.json();
            if (!resp.ok || !res.ok || !res.file_path || !res.url) {
              throw new Error(res?.error || 'Máy chủ không lưu được tệp tải lên');
            }
            if (this.mediaUploadSequence.get(node.id) !== uploadSequence) return;

            if (!node.params) node.params = {};
            node.params.preview_url = res.url;
            if (config.type === 'video' || file.type.startsWith('video/')) {
              node.params.video_path = res.file_path;
              if (node.type === 'product_extractor' || node.type === 'background_remover') {
                node.params.source_video = res.file_path;
                node.params.source_image = res.file_path;
              }
            } else if (node.type === 'character_input') {
              node.params.model_path = res.file_path;
              node.params.character_image = res.file_path;
            } else if (node.type === 'product_extractor' || node.type === 'background_remover') {
              node.params.source_image = res.file_path;
            } else {
              node.params.product_image = res.file_path;
            }
            this.uploadErrors.delete(node.id);
            this.updateNodePreview(node.id, res.url);
          })();

          this.pendingUploads.set(node.id, uploadTask);
          try {
            await uploadTask;
          } catch (uploadErr) {
            if (this.mediaUploadSequence.get(node.id) === uploadSequence) {
              this.uploadErrors.set(node.id, uploadErr.message || 'Tải tệp lên thất bại');
              console.warn('Lỗi tải file lên server', uploadErr);
              window.showAppNotice(`Không tải được tệp lên máy chủ: ${uploadErr.message || uploadErr}. Workflow chưa thể chạy bằng tệp mới.`);
            }
          } finally {
            if (this.pendingUploads.get(node.id) === uploadTask) {
              this.pendingUploads.delete(node.id);
            }
          }
        });

        uploadBtn.addEventListener('click', (e) => {
          e.stopPropagation();
          fileInput.click();
        });

        actions.appendChild(uploadBtn);
        previewContainer.appendChild(fileInput);
      }

      previewContainer.appendChild(actions);

      // Clicking preview container opens lightbox
      previewContainer.addEventListener('click', (e) => {
        if (e.target.closest('button') || e.target.closest('input')) return;
        e.stopPropagation();
        this.selectNode(node.id);
        if (config.url) {
          this.openLightbox(node.title || 'Preview', mediaURL, config.type);
        }
      });

      return previewContainer;
    }

    getNodePreviewConfig(node) {
      const type = node.type;
      const params = node.params || {};

      // 1. Chỉ các node đầu vào (Input Asset Nodes) mới hiển thị ảnh/video nguồn trước khi chạy workflow
      if (type === 'character_input') {
        const inputUrl = params.preview_url || params.model_path || params.character_image || '';
        if (!inputUrl) return null;
        return {
          type: 'image',
          url: inputUrl,
          badge: '👤 Nhân vật mẫu',
          canUpload: true,
          canFetchUrl: true,
          isCutout: false
        };
      }

      if (type === 'product_input') {
        const inputUrl = params.preview_url || params.product_image || '';
        if (!inputUrl) return null;
        return {
          type: 'image',
          url: inputUrl,
          badge: '🛍️ Sản phẩm mẫu',
          canUpload: true,
          canFetchUrl: true,
          isCutout: false
        };
      }

      if (type === 'video_source') {
        const inputUrl = params.preview_url || params.video_path || '';
        if (!inputUrl) return null;
        return {
          type: 'video',
          url: inputUrl,
          badge: '🎬 Video TikTok/Reels gốc',
          canUpload: true,
          canFetchUrl: true,
          isCutout: false
        };
      }

      // 2. Tất cả các node xử lý, 3D, AI, Preview Node:
      // CHỈ CÓ PREVIEW SAU KHI NODE ĐÃ CHẠY XONG VÀ CÓ KẾT QUẢ THỰC TẾ (ComfyUI standard)
      // Trước khi chạy workflow, hoàn toàn không hiển thị khung preview để canvas gọn gàng
      if (node.state && node.state.status === 'completed' && node.state.output_preview) {
        const fn = this.getFilename(node.state.output_preview);
        const url = `/media/${encodeURIComponent(fn)}`;
        const isVid = fn.endsWith('.mp4') || fn.endsWith('.webm') || fn.endsWith('.mov');
        const isCutout = fn.endsWith('.png');

        let badge = '✓ Kết quả thực thi';
        if (type.startsWith('preview_')) {
          badge = '👁️ ComfyUI Preview';
        } else if (type === 'product_extractor' || type === 'background_remover') {
          badge = '✂️ Bóc tách sản phẩm (AI Matting)';
        } else if (type === 'muse_ai_neural_render') {
          badge = '🎬 Video tạo bằng Muse AI';
        } else if (type === 'cinematic_color_grading') {
          badge = '🎨 Cân chỉnh tương phản và bão hòa';
        } else if (type === 'video_master_output' || type === 'video_output') {
          badge = '🎬 Master Video 9:16';
        } else if (type === 'virtual_tryon') {
          badge = '👗 Thay trang phục AI';
        } else if (type === 'image_composite') {
          badge = '✨ Ghép phối cảnh AI';
        } else if (type === 'dance_motion_extractor') {
          badge = '💃 Bóc tách vũ đạo (Pose & Beat)';
        } else if (type === 'dance_retarget') {
          badge = '🕺 Ghép vũ đạo nhân vật';
        } else if (type === 'scene_environment') {
          badge = '🏛️ Không gian / Bối cảnh';
        } else if (type === 'lighting_atmosphere') {
          badge = '☀️ Thời gian & Ánh sáng';
        } else if (type === 'custom_universal_ai') {
          badge = '🤖 AI Đa Năng (Universal)';
        } else if (type === 'custom_media_input') {
          badge = '📥 Media Đầu Vào Tùy Biến';
        } else if (type === 'custom_vision_analyzer') {
          badge = '👁️ Phân Tích Thị Giác';
        } else if (type === 'custom_media_processor') {
          badge = '⚙️ Xử Lý Media FFmpeg';
        } else if (type === 'custom_export_publish') {
          badge = '📤 Xuất Bản Vạn Năng';
        }

        return {
          type: isVid ? 'video' : (isCutout ? 'cutout' : 'image'),
          url: url,
          badge: badge,
          canUpload: false,
          canFetchUrl: false,
          isCutout: isCutout,
          isDedicatedPreview: type.startsWith('preview_')
        };
      }

      // Trước khi chạy: không hiển thị preview
      return null;
    }

    getConnectedInputPreview(nodeId) {
      if (!this.graph.edges) return null;
      const incomingEdges = this.graph.edges.filter(e => (e.to_node || e.toNode || e.ToNode) === nodeId);
      if (!incomingEdges.length) return null;
      for (const incomingEdge of incomingEdges) {
        const fromNodeId = incomingEdge.from_node || incomingEdge.fromNode || incomingEdge.FromNode;
        const sourceNode = this.graph.nodes.find(n => n.id === fromNodeId);
        if (sourceNode) {
          if (sourceNode.state?.output_preview) {
            return `/media/${this.getFilename(sourceNode.state.output_preview)}`;
          }
          if (sourceNode.params?.preview_url) {
            return sourceNode.params.preview_url;
          }
        }
      }
      return null;
    }

    isInputNodeType(type) {
      return type === 'character_input' ||
             type === 'product_input' ||
             type === 'video_source' ||
             type === 'audio_source' ||
             type === 'custom_media_input' ||
             type === 'character_identity';
    }

    propagatePreviewUpdate(nodeId, newUrl) {
      // TUYỆT ĐỐI KHÔNG lan truyền preview sang các node downstream.
      // Mỗi node có preview hoàn toàn độc lập: node đầu vào hiển thị ảnh upload của node đó,
      // các node xử lý/AI chỉ hiển thị kết quả sau khi node đó chạy xong.
      return;
    }

    updateNodePreview(nodeId, newUrl) {
      this.updateNodeVisualPreview(nodeId, newUrl);
    }

    updateNodeVisualPreview(nodeId, outputPreviewOrUrl) {
      if (!outputPreviewOrUrl) return;
      const node = this.graph.nodes.find(n => n.id === nodeId);
      if (!node) return;

      const isInput = this.isInputNodeType(node.type);
      if (!isInput && (!node.state || node.state.status !== 'completed')) {
        const existingPreview = document.getElementById(`node-preview-${nodeId}`);
        if (existingPreview) existingPreview.remove();
        return;
      }

      const url = workflowMediaURL(outputPreviewOrUrl);
      if (!url) return;

      const isVid = url.endsWith('.mp4') || url.endsWith('.webm') || url.endsWith('.mov');
      const isCutout = url.endsWith('.png');

      const nodeEl = document.getElementById(`node-${nodeId}`);
      if (!nodeEl) return;

      let previewContainer = document.getElementById(`node-preview-${nodeId}`);
      if (!previewContainer) {
        previewContainer = document.createElement('div');
        previewContainer.className = `wf-node-preview ${isCutout ? 'preview-cutout' : ''} ${node.type && node.type.startsWith('preview_') ? 'preview-dedicated' : ''}`;
        previewContainer.id = `node-preview-${nodeId}`;

        const footer = document.getElementById(`node-footer-${nodeId}`);
        if (footer) {
          nodeEl.insertBefore(previewContainer, footer);
        } else {
          nodeEl.appendChild(previewContainer);
        }
      }

      previewContainer.innerHTML = '';
      if (isCutout) {
        previewContainer.classList.add('preview-cutout');
      } else {
        previewContainer.classList.remove('preview-cutout');
      }

      // Badge
      const badge = document.createElement('div');
      badge.className = 'wf-preview-badge';
      if (node.type && node.type.startsWith('preview_')) {
        badge.innerHTML = '👁️ ComfyUI Preview';
      } else if (isVid) {
        badge.innerHTML = '▶ Video kết quả';
      } else {
        badge.innerHTML = '✓ Kết quả thực thi';
      }
      previewContainer.appendChild(badge);

      if (isVid) {
        const video = document.createElement('video');
        video.src = url;
        video.muted = true;
        video.loop = true;
        video.playsInline = true;
        video.autoplay = true;
        video.style.maxWidth = '100%';
        video.style.borderRadius = '6px';
        video.onloadedmetadata = () => { this.renderWires(); };
        video.onerror = () => { video.style.display = 'none'; };
        previewContainer.appendChild(video);
      } else {
        const img = document.createElement('img');
        img.src = url;
        img.alt = '';
        img.style.maxWidth = '100%';
        img.style.borderRadius = '6px';
        img.onload = () => { this.renderWires(); };
        img.onerror = () => { img.style.display = 'none'; };
        previewContainer.appendChild(img);
      }

      // Quick actions: Zoom + Download
      const actions = document.createElement('div');
      actions.className = 'wf-preview-actions';

      const zoomBtn = document.createElement('button');
      zoomBtn.type = 'button';
      zoomBtn.className = 'wf-preview-btn';
      zoomBtn.title = 'Phóng to xem chi tiết';
      zoomBtn.innerHTML = '🔍 Xem';
      zoomBtn.onclick = (e) => {
        e.stopPropagation();
        this.openLightbox(node.title || 'Preview', url, isVid ? 'video' : 'image');
      };
      actions.appendChild(zoomBtn);

      const dlBtn = document.createElement('button');
      dlBtn.type = 'button';
      dlBtn.className = 'wf-preview-btn';
      dlBtn.title = 'Tải file kết quả về máy';
      dlBtn.innerHTML = '💾 Tải';
      dlBtn.onclick = (e) => {
        e.stopPropagation();
        const fn = this.getFilename(url) || 'result_preview';
        const a = document.createElement('a');
        a.href = url;
        a.download = fn;
        document.body.appendChild(a);
        a.click();
        document.body.removeChild(a);
      };
      actions.appendChild(dlBtn);

      previewContainer.appendChild(actions);

      previewContainer.onclick = (e) => {
        if (e.target.closest('button')) return;
        e.stopPropagation();
        this.selectNode(nodeId);
        this.openLightbox(node.title || 'Preview', url, isVid ? 'video' : 'image');
      };

      // Cập nhật inspector preview nếu đang chọn node này
      if (this.selectedNodeId === nodeId) {
        const previewBox = document.getElementById('inspector-preview-box');
        const previewStatus = document.getElementById('inspector-preview-status');
        if (previewBox) {
          previewBox.classList.add('has-content');
          if (previewStatus) previewStatus.innerText = 'Đã có kết quả';
          const mediaURL = safeMediaURL(url);
          if (!mediaURL) return;
          const media = document.createElement(isVid ? 'video' : 'img');
          media.src = mediaURL;
          media.style.maxWidth = '100%';
          media.style.borderRadius = '6px';
          if (isVid) {
            media.controls = true;
            media.autoplay = true;
            media.muted = true;
            media.loop = true;
            media.playsInline = true;
          } else {
            media.alt = '';
          }
          media.onerror = () => { media.style.display = 'none'; };
          previewBox.replaceChildren(media);
        }
      }
    }

    propagatePreviewToOutputs(nodeId, url) {
      if (!this.graph.edges) return;
      const outgoing = this.graph.edges.filter(e => (e.from_node || e.fromNode || e.FromNode) === nodeId);
      outgoing.forEach(edge => {
        const toNodeId = edge.to_node || edge.toNode || edge.ToNode;
        const targetNode = this.graph.nodes.find(n => n.id === toNodeId);
        if (targetNode && (targetNode.type === 'preview_media' || targetNode.type === 'preview_image' || targetNode.type === 'preview_video')) {
          if (!targetNode.params) targetNode.params = {};
          targetNode.params.preview_url = url;
          this.updateNodeVisualPreview(toNodeId, url);
        }
      });
    }

    addPreviewNode() {
      const selected = this.graph.nodes.find(n => n.id === this.selectedNodeId);
      let posX = 450, posY = 250;
      if (selected) {
        posX = selected.x + 320;
        posY = selected.y;
      } else {
        const vpRect = this.viewport.getBoundingClientRect();
        posX = (-this.panX + vpRect.width / 2) / this.scale - 120;
        posY = (-this.panY + vpRect.height / 2) / this.scale - 100;
      }

      const id = `node_preview_${Date.now()}`;
      const newNode = {
        id: id,
        type: 'preview_media',
        title: 'Xem trước kết quả (Preview Media)',
        x: Math.round(posX),
        y: Math.round(posY),
        inputs: [{ id: 'media_in', name: 'Ảnh/Video cần xem', type: 'any', required: true }],
        outputs: [{ id: 'media_out', name: 'Truyền tiếp', type: 'any' }],
        params: {},
        state: { status: 'idle', progress: 0 }
      };

      this.graph.nodes.push(newNode);

      // Tự động nối dây từ output của node đang chọn sang Preview Node
      if (selected && selected.outputs && selected.outputs.length > 0) {
        const outPort = selected.outputs[0].id;
        this.addEdge(selected.id, outPort, id, 'media_in');
        if (selected.state?.output_preview) {
          newNode.params.preview_url = `/media/${this.getFilename(selected.state.output_preview)}`;
        } else if (selected.params?.preview_url) {
          newNode.params.preview_url = selected.params.preview_url;
        }
      }

      this.render();
      this.selectNode(id);
    }

    async fetchMediaFromUrl(node, targetUrl, preferredType, paramKey = '') {
      if (!targetUrl || !targetUrl.trim()) return;
      targetUrl = targetUrl.trim();

      if (node.type === 'video_source') {
        if (!node.params) node.params = {};
        node.params.video_url = targetUrl;
        this.clearVideoSourceOtherParam(node, 'video_url');
        this.render();
        this.selectNode(node.id);
        return;
      }

      const previewEl = document.getElementById(`node-preview-${node.id}`);
      let prevHtml = '';
      if (previewEl) {
        prevHtml = previewEl.innerHTML;
        previewEl.innerHTML = `<div style="padding:16px 8px;text-align:center;font-size:11px;color:#38bdf8;background:rgba(15,23,42,0.65);border-radius:6px;line-height:1.4;">⏳ Đang tải media từ link...<br><span style="font-size:10px;opacity:0.8;">(TikTok, Reels, IG, Web)</span></div>`;
      }

      try {
        const resp = await fetch('/api/flow/fetch-url', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            url: targetUrl,
            type: preferredType || 'auto'
          })
        });
        const res = await resp.json();
        if (!resp.ok || !res.ok) {
          throw new Error(res.error || 'Không thể tải media từ URL');
        }

        const mediaType = res.type || res.media_type;
        if (!['image', 'video', 'audio'].includes(mediaType)) {
          throw new Error('Máy chủ không xác định được loại media từ URL');
        }
        if (preferredType && preferredType !== 'auto' && mediaType !== preferredType) {
          throw new Error(`Link trả về ${mediaType || 'media không rõ loại'}, cần ${preferredType}`);
        }

        if (!node.params) node.params = {};
        if (!paramKey) node.params.url = targetUrl;
        const filePath = res.file_path;

        if (mediaType === 'audio') {
          if (node.type === 'audio_source') {
            node.params.audio_path = filePath;
            node.params.audio_url = targetUrl;
          } else if (node.type === 'custom_media_input') {
            node.params.media_path = filePath;
            node.params.media_url = targetUrl;
          } else {
            const pathKey = paramKey.endsWith('_url') ? paramKey.replace(/_url$/, '_path') : paramKey;
            if (pathKey) node.params[pathKey] = filePath;
          }
          node.params.preview_url = res.url;
        } else if (mediaType === 'video') {
          if (node.type === 'custom_media_input') {
            node.params.media_path = filePath;
            node.params.media_url = targetUrl;
          } else if (node.type === 'product_extractor' || node.type === 'background_remover') {
            node.params.source_video = filePath;
          } else {
            const pathKey = paramKey.endsWith('_url') ? paramKey.replace(/_url$/, '_path') : '';
            node.params[pathKey || 'video_path'] = filePath;
            if (paramKey.endsWith('_url')) node.params[paramKey] = targetUrl;
          }
          node.params.preview_url = res.thumbnail_url || res.url;
        } else {
          if (node.type === 'character_input') {
            node.params.model_path = filePath;
            node.params.character_image = filePath;
          } else if (node.type === 'product_extractor' || node.type === 'background_remover') {
            node.params.source_image = filePath;
          } else if (node.type === 'custom_media_input') {
            node.params.media_path = filePath;
            node.params.media_url = targetUrl;
          } else {
            const pathKey = paramKey.endsWith('_url') ? paramKey.replace(/_url$/, '_path') : paramKey;
            node.params[pathKey || 'product_image'] = filePath;
          }
          node.params.preview_url = res.url;
        }

        this.render();
        this.selectNode(node.id);
      } catch (err) {
        console.error('Lỗi nạp link media:', err);
        if (previewEl && prevHtml) {
          previewEl.innerHTML = prevHtml;
        }
        window.showAppNotice(`Không thể tải media từ link: ${err.message || err}`);
      }
    }

    openLightbox(title, url, type) {
      url = safeMediaURL(url);
      if (!url) return;
      let modal = document.getElementById('wf-lightbox-modal');
      if (!modal) {
        modal = document.createElement('div');
        modal.id = 'wf-lightbox-modal';
        modal.className = 'wf-lightbox-modal';
        modal.innerHTML = `
          <div class="wf-lightbox-content">
            <div class="wf-lightbox-header">
              <span class="wf-lightbox-title" id="wf-lightbox-title">Xem chi tiết</span>
              <button type="button" class="wf-lightbox-close" id="wf-lightbox-close">&times;</button>
            </div>
            <div class="wf-lightbox-media" id="wf-lightbox-media"></div>
          </div>
        `;
        document.body.appendChild(modal);

        modal.querySelector('#wf-lightbox-close').addEventListener('click', () => {
          modal.classList.remove('is-open');
          const video = modal.querySelector('video');
          if (video) video.pause();
        });
        modal.addEventListener('click', (e) => {
          if (e.target === modal) {
            modal.classList.remove('is-open');
            const video = modal.querySelector('video');
            if (video) video.pause();
          }
        });
      }

      document.getElementById('wf-lightbox-title').innerText = title;
      const mediaBox = document.getElementById('wf-lightbox-media');
      mediaBox.innerHTML = '';
      if (type === 'video') {
        const v = document.createElement('video');
        v.src = url;
        v.controls = true;
        v.autoplay = true;
        v.playsInline = true;
        mediaBox.appendChild(v);
      } else {
        const img = document.createElement('img');
        img.src = url;
        mediaBox.appendChild(img);
      }
      modal.classList.add('is-open');
    }

    renderWires() {
      while (this.svgLayer.firstChild) {
        this.svgLayer.removeChild(this.svgLayer.firstChild);
      }
      if (!this.graph.edges || !Array.isArray(this.graph.edges)) return;

      this.graph.edges.forEach(edge => {
        const fromNodeId = edge.from_node || edge.fromNode || edge.FromNode;
        const fromPortId = edge.from_port || edge.fromPort || edge.FromPort;
        const toNodeId = edge.to_node || edge.toNode || edge.ToNode;
        const toPortId = edge.to_port || edge.toPort || edge.ToPort;

        const fromDot = this.findPortDot(fromNodeId, fromPortId, true);
        const toDot = this.findPortDot(toNodeId, toPortId, false);
        if (!fromDot || !toDot) return;

        const p1 = this.getPortCoordinates(fromDot);
        const p2 = this.getPortCoordinates(toDot);
        const d = this.calculateBezier(p1.x, p1.y, p2.x, p2.y);

        const fromNode = this.graph.nodes ? this.graph.nodes.find(n => n.id === fromNodeId) : null;
        const toNode = this.graph.nodes ? this.graph.nodes.find(n => n.id === toNodeId) : null;
        const portType = fromDot.parentElement ? (fromDot.parentElement.getAttribute('data-type') || fromDot.parentElement.dataset?.type || 'any') : 'any';

        const isRunning = (fromNode?.state?.status === 'running') || (toNode?.state?.status === 'running');

        const group = document.createElementNS('http://www.w3.org/2000/svg', 'g');
        group.setAttribute('class', `flow-wire-group ${isRunning ? 'active-flowing' : ''}`);
        group.setAttribute('data-edge-id', edge.id || '');

        // 1. Shadow / Glow outline path (ComfyUI & n8n depth)
        const shadowPath = document.createElementNS('http://www.w3.org/2000/svg', 'path');
        shadowPath.setAttribute('d', d);
        shadowPath.setAttribute('class', 'flow-wire-shadow');
        group.appendChild(shadowPath);

        // 2. Core colored wire path
        const wirePath = document.createElementNS('http://www.w3.org/2000/svg', 'path');
        wirePath.setAttribute('d', d);
        wirePath.setAttribute('class', `flow-wire port-type-${portType} ${isRunning ? 'wire-active' : ''}`);
        group.appendChild(wirePath);

        // 3. Flowing stream pulse path if connected nodes are running
        if (isRunning) {
          const pulsePath = document.createElementNS('http://www.w3.org/2000/svg', 'path');
          pulsePath.setAttribute('d', d);
          pulsePath.setAttribute('class', 'flow-wire-pulse');
          group.appendChild(pulsePath);
        }

        // 4. Hit area for easy clicking / hover
        const hitPath = document.createElementNS('http://www.w3.org/2000/svg', 'path');
        hitPath.setAttribute('d', d);
        hitPath.setAttribute('class', 'flow-wire-hit');
        hitPath.addEventListener('click', async (e) => {
          e.stopPropagation();
          if (await window.showAppConfirm('Bạn có muốn xóa kết nối dây này?', { title: 'Xóa kết nối', confirmText: 'Xóa', danger: true })) {
            this.deleteEdge(edge.id);
          }
        });
        group.appendChild(hitPath);

        this.svgLayer.appendChild(group);
      });
    }

    createTempWire(x, y) {
      this.tempWire = document.createElementNS('http://www.w3.org/2000/svg', 'path');
      this.tempWire.setAttribute('class', 'flow-wire temp-wire');
      this.svgLayer.appendChild(this.tempWire);
    }

    updateTempWire(clientX, clientY) {
      if (!this.tempWire || !this.activeConnecting) return;
      const vpRect = this.viewport.getBoundingClientRect();
      const curX = (clientX - vpRect.left) / this.scale;
      const curY = (clientY - vpRect.top) / this.scale;

      const d = this.calculateBezier(this.activeConnecting.startX, this.activeConnecting.startY, curX, curY);
      this.tempWire.setAttribute('d', d);
    }

    cancelConnecting() {
      if (this.tempWire) {
        this.tempWire.remove();
        this.tempWire = null;
      }
      this.activeConnecting = null;
    }

    calculateBezier(x1, y1, x2, y2) {
      const dx = x2 - x1;
      const dy = y2 - y1;

      if (dx >= 20) {
        // Luồng xuôi tự nhiên từ trái sang phải
        const tension = Math.min(Math.max(45, dx * 0.5), 180);
        return `M ${x1} ${y1} C ${x1 + tension} ${y1}, ${x2 - tension} ${y2}, ${x2} ${y2}`;
      } else {
        // Luồng quẹo ngược về sau (Loopback wire): uốn cong mượt mà hình chữ S, không bị gập góc
        const dist = Math.hypot(dx, dy);
        const loopOffset = Math.min(180, Math.max(65, dist * 0.35));
        const cyOffset = dy >= 0 ? Math.min(60, Math.max(25, Math.abs(dy) * 0.2)) : -Math.min(60, Math.max(25, Math.abs(dy) * 0.2));
        return `M ${x1} ${y1} C ${x1 + loopOffset} ${y1 + cyOffset}, ${x2 - loopOffset} ${y2 - cyOffset}, ${x2} ${y2}`;
      }
    }

    findPortDot(nodeId, portId, isOutput) {
      if (!nodeId) return null;
      const ports = Array.from(this.nodesLayer.querySelectorAll('.wf-port')).filter(port =>
        port.dataset.nodeId === String(nodeId) && port.dataset.isOutput === (isOutput ? '1' : '0')
      );

      // 1. Tìm chính xác theo data-node-id và data-port-id
      if (portId) {
        const exactPort = ports.find(port => port.dataset.portId === String(portId));
        const dot = exactPort && exactPort.querySelector('.wf-port-dot');
        if (dot) return dot;
      }

      // 2. Tìm kiếm linh hoạt theo case-insensitive hoặc alias phổ biến
      if (portId && ports.length > 0) {
        const pLower = portId.toLowerCase().replace(/[-_]/g, '');
        for (const p of ports) {
          const curId = (p.dataset.portId || '').toLowerCase().replace(/[-_]/g, '');
          if (curId === pLower || curId.includes(pLower) || pLower.includes(curId)) {
            const dot = p.querySelector('.wf-port-dot');
            if (dot) return dot;
          }
        }
      }

      // 3. Fallback thông minh: nếu node chỉ có cổng cùng chiều, tự động gán vào cổng khả dụng đầu tiên
      if (ports.length > 0) {
        const fallbackDot = ports[0].querySelector('.wf-port-dot');
        if (fallbackDot) return fallbackDot;
      }

      return null;
    }

    getPortCoordinates(dotEl) {
      if (!dotEl) return { x: 0, y: 0 };
      const nodeEl = dotEl.closest('.wf-node');
      if (nodeEl) {
        const nx = parseFloat(nodeEl.style.left) || 0;
        const ny = parseFloat(nodeEl.style.top) || 0;
        const nodeRect = nodeEl.getBoundingClientRect();
        const dotRect = dotEl.getBoundingClientRect();
        const relX = (dotRect.left + dotRect.width / 2 - nodeRect.left) / this.scale;
        const relY = (dotRect.top + dotRect.height / 2 - nodeRect.top) / this.scale;
        return {
          x: Math.round(nx + relX),
          y: Math.round(ny + relY)
        };
      }

      const dotRect = dotEl.getBoundingClientRect();
      const vpRect = this.viewport.getBoundingClientRect();
      return {
        x: (dotRect.left + dotRect.width / 2 - vpRect.left) / this.scale,
        y: (dotRect.top + dotRect.height / 2 - vpRect.top) / this.scale
      };
    }

    addEdge(fromNode, fromPort, toNode, toPort) {
	  const source = this.graph.nodes.find(n => n.id === fromNode);
	  const target = this.graph.nodes.find(n => n.id === toNode);
	  const sourcePort = source && this.getNodePorts(source, true).find(p => p.id === fromPort);
	  const targetPort = target && this.getNodePorts(target, false).find(p => p.id === toPort);
	  if (!sourcePort || !targetPort) {
	    window.showAppNotice('Không thể nối dây: cổng không còn tồn tại. Tải lại contract workflow con.');
	    return false;
	  }
	  if (!this.isPortTypeCompatible(sourcePort.type, targetPort.type)) {
	    window.showAppNotice(`Không thể nối ${sourcePort.type} vào ${targetPort.type}. Chọn cổng cùng kiểu dữ liệu.`);
	    return false;
	  }

      // Remove any existing edge to the same input port
      this.graph.edges = this.graph.edges.filter(e => !(e.to_node === toNode && e.to_port === toPort));

      const newEdge = {
        id: `e_${Date.now()}_${Math.floor(Math.random() * 1000)}`,
        from_node: fromNode,
        from_port: fromPort,
        to_node: toNode,
        to_port: toPort
      };
      this.graph.edges.push(newEdge);
      this.renderWires();

      // ComfyUI: Khi nối dây vào node Preview, lập tức chuyển kết quả xem trước sang
      const targetNode = this.graph.nodes.find(n => n.id === toNode);
      if (targetNode && (targetNode.type === 'preview_media' || targetNode.type === 'preview_image' || targetNode.type === 'preview_video')) {
        const sourceNode = this.graph.nodes.find(n => n.id === fromNode);
        if (sourceNode) {
          const previewUrl = sourceNode.state?.output_preview ? `/media/${this.getFilename(sourceNode.state.output_preview)}` : (sourceNode.params?.preview_url || '');
          if (previewUrl) {
            targetNode.params = targetNode.params || {};
            targetNode.params.preview_url = previewUrl;
            this.updateNodeVisualPreview(toNode, previewUrl);
          }
        }
      }
      return true;
    }

    deleteEdge(edgeId) {
      this.graph.edges = this.graph.edges.filter(e => e.id !== edgeId);
      this.renderWires();
    }

    deleteNode(nodeId) {
      this.graph.nodes = this.graph.nodes.filter(n => n.id !== nodeId);
      this.graph.edges = this.graph.edges.filter(e => {
        const fromNode = e.from_node || e.fromNode || e.FromNode;
        const toNode = e.to_node || e.toNode || e.ToNode;
        return fromNode !== nodeId && toNode !== nodeId;
      });
      if (this.selectedNodeId === nodeId) {
        this.deselectAll();
      }
      this.render();
    }

    createCanvasClipboardPayload() {
      const selectedNode = this.selectedNodeId && this.graph.nodes.find(node => node.id === this.selectedNodeId);
      const nodes = selectedNode ? [selectedNode] : (this.graph.nodes || []);
      if (!nodes.length) return null;
      const copiedIDs = new Set(nodes.map(node => String(node.id)));
      const clonedNodes = nodes.map(node => {
        const clone = JSON.parse(JSON.stringify(node));
        delete clone.state;
        return clone;
      });
      const edges = (this.graph.edges || [])
        .filter(edge => copiedIDs.has(String(edge.from_node)) && copiedIDs.has(String(edge.to_node)))
        .map(edge => JSON.parse(JSON.stringify(edge)));
      return {
        format: 'miunova-workflow-clipboard',
        version: 1,
        kind: selectedNode ? 'nodes' : 'workflow',
        source_graph_id: this.graph.id || '',
        nodes: clonedNodes,
        edges
      };
    }

    parseCanvasClipboard(text) {
      if (typeof text !== 'string' || !text.startsWith(FLOW_CLIPBOARD_PREFIX)) return null;
      try {
        const payload = JSON.parse(text.slice(FLOW_CLIPBOARD_PREFIX.length));
        if (payload?.format !== 'miunova-workflow-clipboard' || payload.version !== 1 || !['nodes', 'workflow'].includes(payload.kind)) return null;
        if (!Array.isArray(payload.nodes) || !payload.nodes.length || !Array.isArray(payload.edges) || payload.nodes.length > 5000 || payload.edges.length > 20000) return null;
        const seenIDs = new Set();
        for (const node of payload.nodes) {
          if (!node || typeof node.id !== 'string' || !node.id || typeof node.type !== 'string' || !node.type || seenIDs.has(node.id)) return null;
          seenIDs.add(node.id);
        }
        for (const edge of payload.edges) {
          if (!edge || typeof edge.from_node !== 'string' || typeof edge.to_node !== 'string' ||
              !seenIDs.has(edge.from_node) || !seenIDs.has(edge.to_node) ||
              typeof edge.from_port !== 'string' || typeof edge.to_port !== 'string') return null;
        }
        return payload;
      } catch (_) {
        return null;
      }
    }

    flashClipboardButton(buttonID, message) {
      const label = document.querySelector(`#${buttonID} [data-clipboard-label]`);
      if (!label) return;
      const original = label.dataset.originalLabel || label.textContent;
      label.dataset.originalLabel = original;
      label.textContent = message;
      window.clearTimeout(label._resetTimer);
      label._resetTimer = window.setTimeout(() => {
        if (label.isConnected) label.textContent = original;
      }, 1500);
    }

    async copyCanvasContent() {
      const payload = this.createCanvasClipboardPayload();
      if (!payload) {
        this.flashClipboardButton('btn-copy-canvas', 'Canvas trống');
        return false;
      }
      const text = FLOW_CLIPBOARD_PREFIX + JSON.stringify(payload);
      try { window.localStorage.setItem(FLOW_CLIPBOARD_STORAGE_KEY, text); } catch (_) {}
      try {
        if (navigator.clipboard?.writeText) await navigator.clipboard.writeText(text);
      } catch (_) {
        // Same-origin local storage remains available as an in-app clipboard fallback.
      }
      this.flashClipboardButton('btn-copy-canvas', payload.kind === 'workflow' ? 'Đã chép workflow' : 'Đã chép node');
      return true;
    }

    async pasteCanvasContent() {
      let text = '';
      try {
        if (navigator.clipboard?.readText) {
          text = await navigator.clipboard.readText();
        }
      } catch (_) {
        // Same-origin local storage remains available as an in-app clipboard fallback.
      }
      let payload = this.parseCanvasClipboard(text);
      if (!payload) {
        try { payload = this.parseCanvasClipboard(window.localStorage.getItem(FLOW_CLIPBOARD_STORAGE_KEY) || ''); } catch (_) {}
      }
      if (!payload) {
        this.flashClipboardButton('btn-paste-canvas', 'Clipboard không có node/flow');
        return false;
      }
      return this.pasteCanvasPayload(payload);
    }

    makePastedID(prefix, usedIDs) {
      let id = '';
      do {
        id = `${prefix}_${Date.now().toString(36)}_${Math.random().toString(36).slice(2, 9)}`;
      } while (usedIDs.has(id));
      usedIDs.add(id);
      return id;
    }

    pasteCanvasPayload(payload) {
      if (!payload || !Array.isArray(this.graph.nodes) || !Array.isArray(this.graph.edges)) return false;
      const nodeIDs = new Set(this.graph.nodes.map(node => String(node.id)));
      const edgeIDs = new Set(this.graph.edges.map(edge => String(edge.id)));
      const remap = new Map();
      payload.nodes.forEach(node => remap.set(node.id, this.makePastedID('node', nodeIDs)));

      const positions = payload.nodes.map(node => ({ x: Number(node.x) || 0, y: Number(node.y) || 0 }));
      const bounds = positions.reduce((box, point) => ({
        left: Math.min(box.left, point.x),
        top: Math.min(box.top, point.y),
        right: Math.max(box.right, point.x + 280),
        bottom: Math.max(box.bottom, point.y + 150)
      }), { left: Infinity, top: Infinity, right: -Infinity, bottom: -Infinity });
      const centerX = (this.container.clientWidth / 2 - this.panX) / this.scale;
      const centerY = (this.container.clientHeight / 2 - this.panY) / this.scale;
      const offsetX = centerX - (bounds.left + bounds.right) / 2;
      const offsetY = centerY - (bounds.top + bounds.bottom) / 2;

      const copiedNodes = payload.nodes.map(node => {
        const clone = JSON.parse(JSON.stringify(node));
        clone.id = remap.get(node.id);
        clone.x = Math.round((Number(node.x) || 0) + offsetX);
        clone.y = Math.round((Number(node.y) || 0) + offsetY);
        clone.state = { status: 'idle', progress: 0 };
        return clone;
      });
      const copiedEdges = payload.edges.flatMap(edge => {
        const fromNode = remap.get(String(edge.from_node));
        const toNode = remap.get(String(edge.to_node));
        if (!fromNode || !toNode) return [];
        const clone = JSON.parse(JSON.stringify(edge));
        clone.id = this.makePastedID('edge', edgeIDs);
        clone.from_node = fromNode;
        clone.to_node = toNode;
        return [clone];
      });

      this.graph.nodes.push(...copiedNodes);
      this.graph.edges.push(...copiedEdges);
      this.render();
      this.selectNode(copiedNodes[0].id);
      const kindLabel = payload.kind === 'workflow' ? 'workflow' : (copiedNodes.length === 1 ? 'node' : `${copiedNodes.length} node`);
      this.flashClipboardButton('btn-paste-canvas', `Đã dán ${kindLabel}`);
      return true;
    }

    addNode(nodeType, x, y) {
      const def = this.definitions[nodeType];
      if (!def) return;

      const id = `node_${Date.now()}`;
      const newNode = {
        id: id,
        type: nodeType,
        title: def.title,
        category: def.category,
        x: Math.round(x || 200),
        y: Math.round(y || 200),
        inputs: def.inputs ? JSON.parse(JSON.stringify(def.inputs)) : [],
        outputs: def.outputs ? JSON.parse(JSON.stringify(def.outputs)) : [],
        params: def.default_params ? JSON.parse(JSON.stringify(def.default_params)) : {},
        state: { status: 'idle', progress: 0 }
      };

      this.graph.nodes.push(newNode);

      // Auto-connect if pending from '+' handle
      if (this.pendingConnect) {
        const { fromNode, fromPort } = this.pendingConnect;
        const matchingInput = this.getNodePorts(newNode, false).find(inp => this.isPortTypeCompatible(this.pendingConnect.portType, inp.type));
        if (matchingInput) {
          this.addEdge(fromNode, fromPort, id, matchingInput.id);
        } else {
          window.showAppNotice('Node đã thêm nhưng không có cổng input tương thích để tự nối.');
        }
        this.pendingConnect = null;
      }

      this.render();
      this.selectNode(id);
    }

    selectNode(nodeId, openDetail = true) {
      if (this.selectedNodeId !== nodeId) this.activeInspectorTab = 'parameters';
      this.selectedNodeId = nodeId;
      this.nodesLayer.querySelectorAll('.wf-node').forEach(el => {
        el.classList.toggle('selected', el.id === `node-${nodeId}`);
      });
      if (openDetail) {
        this.openInspector();
        this.updateInspector();
      }
    }

    deselectAll() {
      this.selectedNodeId = null;
      this.nodesLayer.querySelectorAll('.wf-node').forEach(el => el.classList.remove('selected'));
      this.closeInspector();
      this.updateInspector();
    }

    openInspector() {
      if (this.inspector) {
        this.inspector.classList.remove('collapsed');
      }
    }

    closeInspector() {
      if (this.inspector) {
        this.inspector.classList.add('collapsed');
      }
      this.selectedNodeId = null;
      this.nodesLayer.querySelectorAll('.wf-node').forEach(el => el.classList.remove('selected'));
    }

    renderNodeStatus(state, nodeType) {
      if (!state || state.status === 'idle') {
        return `<span class="wf-status-badge wf-status-idle">Chờ thực thi</span>`;
      }
      if (state.status === 'queued') {
        return `<span class="wf-status-badge wf-status-idle">⏳ Đang xếp hàng...</span>`;
      }
      if (state.status === 'running') {
        const progress = Math.max(0, Math.min(100, Number(state.progress) || 20));
        return `<span class="wf-status-badge wf-status-running"><span class="wf-spinner"></span> ⚡ Đang thực thi (${progress}%)...</span>`;
      }
      if (state.status === 'completed') {
        return `<div class="wf-status-badge wf-status-completed">✓ Hoàn tất 100%</div>`;
      }
      if (state.status === 'failed') {
        const error = String(state.error || '');
        return `<span class="wf-status-badge wf-status-failed" title="${escapeHTML(error)}">⚠️ Lỗi: ${escapeHTML(error ? error.slice(0, 32) : 'Thất bại')}</span>`;
      }
      if (state.status === 'cancelled') {
        return `<span class="wf-status-badge wf-status-cancelled">⏹ Đã dừng</span>`;
      }
      return '';
    }

    isVisualNodeType(type) {
      if (!type) return false;
      const visualTypes = [
        'character_input', 'product_input', 'product_extractor', 'background_remover',
        'image_composite', 'virtual_tryon', 'google_flow_video', 'muse_ai_video',
        'video_source', 'video_upscale', 'concat_scenes', 'video_output', 'image_output',
        'muse_ai_neural_render', 'cinematic_color_grading', 'video_master_output',
        'preview_media', 'preview_image', 'preview_video',
        'scene_environment', 'lighting_atmosphere', 'dance_motion_extractor', 'dance_retarget',
        'custom_universal_ai', 'custom_media_input', 'custom_vision_analyzer', 'custom_media_processor', 'custom_export_publish'
      ];
      return visualTypes.includes(type);
    }

    getFilename(path) {
      if (!path) return '';
      return path.split('/').pop();
    }

    graphFingerprint() {
      const snapshot = JSON.parse(JSON.stringify(this.graph || {}));
      delete snapshot.created_at;
      delete snapshot.updated_at;
      (snapshot.nodes || []).forEach(node => delete node.state);
      return JSON.stringify(snapshot);
    }

    isGraphDirty() {
      return this.graphFingerprint() !== this.savedGraphFingerprint;
    }

    async confirmDiscardGraphChanges() {
      return !this.isGraphDirty() || await window.showAppConfirm('Workflow có thay đổi chưa lưu. Bỏ thay đổi và tiếp tục?', { title: 'Thay đổi chưa lưu', confirmText: 'Bỏ thay đổi', danger: true });
    }

    syncGraphHeader() {
      const nameInput = document.getElementById('wf-active-name');
      const categorySelect = document.getElementById('wf-active-category');
      const presetPill = document.getElementById('wf-preset-pill');
      if (nameInput) nameInput.value = this.graph.name || '';
      if (presetPill) {
        presetPill.style.display = this.isPresetWorkflow(this.graph.id) ? 'inline-flex' : 'none';
      }
      if (categorySelect) {
        const category = this.graph.category || 'custom';
        if (![...categorySelect.options].some(option => option.value === category)) {
          const option = document.createElement('option');
          option.value = category;
          option.innerText = category;
          categorySelect.appendChild(option);
        }
        categorySelect.value = category;
      }
    }

    syncGraphMetadataFromHeader() {
      const nameInput = document.getElementById('wf-active-name');
      const categorySelect = document.getElementById('wf-active-category');
      if (nameInput) this.graph.name = nameInput.value.trim();
      if (categorySelect) this.graph.category = categorySelect.value;
    }

    bindGraphMetadata() {
      const nameInput = document.getElementById('wf-active-name');
      const categorySelect = document.getElementById('wf-active-category');
      if (nameInput) nameInput.addEventListener('input', () => { this.graph.name = nameInput.value; });
      if (categorySelect) categorySelect.addEventListener('change', () => { this.graph.category = categorySelect.value; });
      this.syncGraphHeader();
    }

    renderWorkflowOutputs(outputs) {
      const panel = document.getElementById('flow-run-outputs');
      if (!panel) return;
      panel.replaceChildren();
      const entries = Object.entries(outputs || {});
      panel.hidden = entries.length === 0;
      entries.forEach(([key, value]) => {
        const row = document.createElement('div');
        row.className = 'wf-run-output';
        const label = document.createElement('strong');
        label.innerText = key;
        row.appendChild(label);
        const mediaPath = this.findOutputMediaPath(value);
        if (mediaPath) {
          const filename = mediaPath.split('/').pop();
          const url = /^https?:\/\//i.test(mediaPath)
            ? mediaPath
            : (mediaPath.startsWith('/media/') ? mediaPath : '/media/' + encodeURIComponent(filename));
          if (/\.(mp4|webm|mov)$/i.test(filename)) {
            const media = document.createElement('video');
            media.src = url;
            media.controls = true;
            row.appendChild(media);
          } else if (/\.(mp3|aac|wav|ogg|m4a|flac)$/i.test(filename)) {
            const media = document.createElement('audio');
            media.src = url;
            media.controls = true;
            row.appendChild(media);
          } else {
            const media = document.createElement('img');
            media.src = url;
            media.alt = key;
            row.appendChild(media);
          }
          const link = document.createElement('a');
          link.href = url;
          link.download = filename;
          link.innerText = 'Tải xuống';
          row.appendChild(link);
        }
        if (!mediaPath || typeof value !== 'string') {
          const valueEl = document.createElement('pre');
          valueEl.innerText = typeof value === 'string' ? value : JSON.stringify(value, null, 2);
          row.appendChild(valueEl);
        }
        panel.appendChild(row);
      });
      const runPanel = document.getElementById('flow-run-panel');
      if (runPanel && entries.length) runPanel.hidden = false;
    }

    findOutputMediaPath(value) {
      const mediaPattern = /\.(mp4|webm|mov|jpg|jpeg|png|webp|mp3|aac|wav|ogg|m4a|flac)$/i;
      if (typeof value === 'string') return mediaPattern.test(value) ? value : '';
      if (Array.isArray(value)) {
        for (const child of value) {
          const path = this.findOutputMediaPath(child);
          if (path) return path;
        }
      } else if (value && typeof value === 'object') {
        for (const child of Object.values(value)) {
          const path = this.findOutputMediaPath(child);
          if (path) return path;
        }
      }
      return '';
    }

    isWorkflowCallType(type) {
      return type === 'workflow_call' || (typeof type === 'string' && type.startsWith('workflow_call_'));
    }

    isWorkflowMapType(type) {
      return type === 'workflow_map';
    }

    getWorkflowPortTypes() {
      return ['any', 'image', 'image_list', 'text_list', 'video', 'audio', 'text', 'product', 'motion', 'number', 'config'];
    }

    getNodePorts(node, isOutput) {
      const def = this.definitions[node.type] || {};
      const params = node.params || {};
      if (node.type === 'workflow_input') {
        return isOutput
          ? [{ id: 'value', name: params.label || 'Giá trị', type: params.value_type || 'any' }]
          : [];
      }
      if (node.type === 'workflow_output') {
        const port = { id: 'value', name: params.label || 'Giá trị', type: params.value_type || 'any' };
        return isOutput ? [port] : [{ ...port, required: true }];
      }
      const nodePorts = isOutput ? node.outputs : node.inputs;
      const defPorts = isOutput ? def.outputs : def.inputs;
      const ports = Array.isArray(nodePorts) && nodePorts.length > 0 ? [...nodePorts] : [...(defPorts || [])];

      // Saved graphs can outlive experimental node definitions. Preserve their
      // persisted edge endpoints in the canvas so they remain visible/editable.
      const edgePortIds = new Set();
      (this.graph.edges || []).forEach(edge => {
        const endpointNode = isOutput
          ? (edge.from_node || edge.fromNode || edge.FromNode)
          : (edge.to_node || edge.toNode || edge.ToNode);
        if (endpointNode !== node.id) return;
        const portId = isOutput
          ? (edge.from_port || edge.fromPort || edge.FromPort)
          : (edge.to_port || edge.toPort || edge.ToPort);
        if (portId) edgePortIds.add(String(portId));
      });
      edgePortIds.forEach(id => {
        if (!ports.some(port => String(port.id) === id)) {
          ports.push({ id, name: id.replace(/_/g, ' '), type: 'any', legacy: true });
        }
      });
      return ports;
    }

    isPortTypeCompatible(from, to) {
      return !from || !to || from === 'any' || to === 'any' || from === to || (from === 'image' && to === 'image_list');
    }

    getCategoryIcon(cat) {
      switch (cat) {
        // 6 Tầng Kiến trúc Studio 3D & VFX Chuyên nghiệp:
        case 'identity': return '👤';
        case 'geometry': return '📐';
        case 'camera': return '🎥';
        case 'lighting': return '💡';
        case 'render': return '🧠';
        case 'post': return '🎨';
        case 'workflow': return '🧩';
        case 'custom': return '✨';
        // Danh mục cơ bản tương thích:
        case 'input': return '📥';
        case 'process': return '⚙️';
        case 'ai': return '🤖';
        case 'output': return '📤';
        default: return '📦';
      }
    }

    bindToolbar() {
      // Open graphs list modal
      const btnOpenGraphs = document.getElementById('btn-open-graphs-modal');
      if (btnOpenGraphs) {
        btnOpenGraphs.addEventListener('click', () => {
          this.openGraphsModal();
        });
      }

      // Create new workflow button
      const btnNew = document.getElementById('btn-new-graph');
      if (btnNew) btnNew.addEventListener('click', async () => this.createNewGraph());

      // Add Node button
      const btnAdd = document.getElementById('btn-add-node');
      if (btnAdd) {
        btnAdd.addEventListener('click', () => {
          this.pendingConnect = null;
          this.openAddNodeModal();
        });
      }

      const btnCopyCanvas = document.getElementById('btn-copy-canvas');
      if (btnCopyCanvas) btnCopyCanvas.addEventListener('click', () => { void this.copyCanvasContent(); });
      const btnPasteCanvas = document.getElementById('btn-paste-canvas');
      if (btnPasteCanvas) btnPasteCanvas.addEventListener('click', () => { void this.pasteCanvasContent(); });

      // Save Graph button
      const btnSave = document.getElementById('btn-save-graph');
      if (btnSave) {
        btnSave.addEventListener('click', async () => {
          this.syncGraphMetadataFromHeader();
          if (!this.graph.name) {
            window.showAppNotice('Nhập tên workflow trước khi lưu.');
            document.getElementById('wf-active-name')?.focus();
            return;
          }
          let name = this.graph.name;
          if (this.graph.is_preset || this.isPresetWorkflow(this.graph.id)) {
            const newName = await window.showAppPrompt('Đây là workflow mẫu (Preset) của hệ thống và được bảo vệ (không thể ghi đè). Vui lòng đặt tên cho bản lưu riêng tùy chỉnh của bạn:', `${this.graph.name} (Bản sao)`, { label: 'Tên workflow', required: true, confirmText: 'Lưu bản sao' });
            if (!newName) return;
            name = newName;
            this.graph.id = `flow_${Date.now()}`;
            this.graph.name = newName;
            this.graph.is_preset = false;
            this.syncGraphHeader();
          }

          try {
            const resp = await fetch('/api/flow/graphs', {
              method: 'POST',
              headers: { 'Content-Type': 'application/json' },
              body: JSON.stringify(this.graph)
            });
            const data = await resp.json();
            if (data.ok) {
              if (data.graph) this.graph = data.graph;
              this.graphPersisted = true;
              this.savedGraphFingerprint = this.graphFingerprint();
              this.syncGraphHeader();
              window.history.pushState(null, '', `/workflow?preset=${encodeURIComponent(this.graph.id)}`);
              window.showAppNotice(`✓ Đã lưu workflow "${this.graph.name}" thành công!`);
              await this.refreshGraphsList();
            } else {
              window.showAppNotice('Lỗi lưu workflow: ' + (data.error || ''));
            }
          } catch (err) {
            window.showAppNotice('Lỗi kết nối máy chủ: ' + err.message);
          }
        });
      }

      // Duplicate Active Graph button
      const btnDupActive = document.getElementById('btn-duplicate-active-graph');
      if (btnDupActive) {
        btnDupActive.addEventListener('click', () => {
          this.duplicateGraph(this.graph.id, this.graph.name);
        });
      }

      // Delete Active Graph button
      const btnDelActive = document.getElementById('btn-delete-active-graph');
      if (btnDelActive) {
        btnDelActive.addEventListener('click', () => {
          if (this.graph.is_preset) {
            window.showAppNotice('Đây là mẫu preset có sẵn của hệ thống. Bạn có thể bấm "Nhân bản" để tạo bản sao riêng và tự do chỉnh sửa hoặc xóa.');
            return;
          }
          this.deleteGraph(this.graph.id, this.graph.name);
        });
      }

      // Run Flow button
      const btnRun = document.getElementById('btn-run-flow');
      if (btnRun) {
        btnRun.addEventListener('click', () => {
          this.runFlow();
        });
      }

      // Cancel Flow button
      const btnCancel = document.getElementById('btn-cancel-flow');
      if (btnCancel) {
        btnCancel.addEventListener('click', () => {
          this.cancelFlow();
        });
      }

      // Zoom buttons
      const btnZoomIn = document.getElementById('btn-zoom-in');
      if (btnZoomIn) {
        btnZoomIn.addEventListener('click', () => {
          this.scale = Math.min(2.5, this.scale * 1.15);
          this.applyTransform();
          this.updateZoomDisplay();
        });
      }

      const btnZoomOut = document.getElementById('btn-zoom-out');
      if (btnZoomOut) {
        btnZoomOut.addEventListener('click', () => {
          this.scale = Math.max(0.35, this.scale / 1.15);
          this.applyTransform();
          this.updateZoomDisplay();
        });
      }

      const btnZoomReset = document.getElementById('btn-zoom-reset');
      if (btnZoomReset) {
        btnZoomReset.addEventListener('click', () => {
          this.scale = 1.0;
          this.panX = 40;
          this.panY = 40;
          this.applyTransform();
          this.updateZoomDisplay();
        });
      }

      const btnFitWorkflow = document.getElementById('btn-fit-workflow');
      if (btnFitWorkflow) btnFitWorkflow.addEventListener('click', () => this.fitCanvas());

      // Theme toggle from workflow toolbar
      const wfThemeToggle = document.getElementById('wf-theme-toggle');
      if (wfThemeToggle) {
        wfThemeToggle.addEventListener('click', () => {
          const doc = document.documentElement;
          const cur = doc.dataset.theme === 'dark' ? 'light' : 'dark';
          doc.dataset.theme = cur;
          try {
            localStorage.setItem('aicos-theme', cur);
          } catch (e) {}
        });
      }

      // Export API button & Modal listeners
      const btnExportApi = document.getElementById('btn-export-api');
      if (btnExportApi) {
        btnExportApi.addEventListener('click', () => {
          this.openExportApiModal();
        });
      }

      const btnCloseApiModal = document.getElementById('btn-close-api-modal');
      if (btnCloseApiModal) {
        btnCloseApiModal.addEventListener('click', () => {
          this.closeExportApiModal();
        });
      }

      const btnCloseApiFooter = document.getElementById('btn-close-api-footer');
      if (btnCloseApiFooter) {
        btnCloseApiFooter.addEventListener('click', () => {
          this.closeExportApiModal();
        });
      }

      const btnCopyApiUrl = document.getElementById('btn-copy-api-url');
      if (btnCopyApiUrl) {
        btnCopyApiUrl.addEventListener('click', () => {
          const urlEl = document.getElementById('api-endpoint-url');
          if (urlEl) {
            navigator.clipboard.writeText(urlEl.innerText).then(() => {
              btnCopyApiUrl.innerText = '✓ Đã chép';
              setTimeout(() => btnCopyApiUrl.innerText = 'Copy URL', 2000);
            });
          }
        });
      }

      const tabBtnCurl = document.getElementById('tab-btn-curl');
      const tabBtnJsonFlat = document.getElementById('tab-btn-json-flat');
      const tabBtnJsonNode = document.getElementById('tab-btn-json-node');
      const tabContentCurl = document.getElementById('api-tab-content-curl');
      const tabContentJsonFlat = document.getElementById('api-tab-content-json-flat');
      const tabContentJsonNode = document.getElementById('api-tab-content-json-node');

      const setApiTab = (tab) => {
        [tabBtnCurl, tabBtnJsonFlat, tabBtnJsonNode].forEach(btn => btn && btn.classList.remove('active'));
        [tabContentCurl, tabContentJsonFlat, tabContentJsonNode].forEach(c => { if (c) c.style.display = 'none'; });

        if (tab === 'curl' && tabBtnCurl && tabContentCurl) {
          tabBtnCurl.classList.add('active');
          tabContentCurl.style.display = 'block';
        } else if (tab === 'json-flat' && tabBtnJsonFlat && tabContentJsonFlat) {
          tabBtnJsonFlat.classList.add('active');
          tabContentJsonFlat.style.display = 'block';
        } else if (tab === 'json-node' && tabBtnJsonNode && tabContentJsonNode) {
          tabBtnJsonNode.classList.add('active');
          tabContentJsonNode.style.display = 'block';
        }
      };

      if (tabBtnCurl) tabBtnCurl.addEventListener('click', () => setApiTab('curl'));
      if (tabBtnJsonFlat) tabBtnJsonFlat.addEventListener('click', () => setApiTab('json-flat'));
      if (tabBtnJsonNode) tabBtnJsonNode.addEventListener('click', () => setApiTab('json-node'));

      const btnCopyCurl = document.getElementById('btn-copy-curl');
      if (btnCopyCurl) {
        btnCopyCurl.addEventListener('click', () => {
          let textToCopy = '';
          if (tabBtnCurl && tabBtnCurl.classList.contains('active')) {
            textToCopy = document.getElementById('api-curl-code')?.innerText || '';
          } else if (tabBtnJsonNode && tabBtnJsonNode.classList.contains('active')) {
            textToCopy = document.getElementById('api-request-body-node')?.value || '';
          } else {
            textToCopy = document.getElementById('api-request-body')?.value || '';
          }
          if (textToCopy) {
            navigator.clipboard.writeText(textToCopy).then(() => {
              btnCopyCurl.innerText = '✓ Đã sao chép';
              setTimeout(() => btnCopyCurl.innerText = 'Sao chép', 2000);
            });
          }
        });
      }

      const btnRunApiTest = document.getElementById('btn-run-api-test');
      if (btnRunApiTest) {
        btnRunApiTest.addEventListener('click', () => {
          this.runApiTest();
        });
      }
    }

    async runFlow() {
      const runBtn = document.getElementById('btn-run-flow');
      const cancelBtn = document.getElementById('btn-cancel-flow');
      const statusPill = this.statusPill;
      const runPanel = document.getElementById('flow-run-panel');

      if (this.pendingUploads.size) {
        if (runBtn) runBtn.disabled = true;
        try {
          while (this.pendingUploads.size) {
            await Promise.allSettled([...this.pendingUploads.values()]);
          }
        } finally {
          if (runBtn) runBtn.disabled = false;
        }
      }
      const failedUpload = this.uploadErrors.values().next().value;
      if (failedUpload) {
        window.showAppNotice(`Sửa lỗi tải tệp trước khi chạy workflow: ${failedUpload}`);
        return;
      }

      let workflowInputs;
      try {
        workflowInputs = await this.collectWorkflowRunInputs();
        if (workflowInputs === null) return;
      } catch (err) {
        window.showAppNotice(`Không thể chạy workflow: ${err.message}`);
        return;
      }

      const providerCreditsAcknowledged = flowGraphMayUseProviderCredits(this.graph);
      if (providerCreditsAcknowledged && !window.confirm('Workflow này có thể gửi nội dung tới Muse AI hoặc Google Flow và dùng credits của tài khoản. Chỉ tiếp tục nếu bạn cho phép dùng credits. Tiếp tục?')) return;

      if (runPanel) runPanel.hidden = false;
      this.renderWorkflowOutputs({});

      if (runBtn) runBtn.disabled = true;
      if (cancelBtn) cancelBtn.style.display = 'inline-flex';
      if (statusPill) {
        statusPill.className = 'wf-status-pill status-running';
        statusPill.innerText = '⚡ Đang chạy workflow...';
      }

      // Reset visual node states to queued and remove previous execution previews from processing nodes
      this.graph.nodes.forEach(n => {
        n.state = { status: 'queued', progress: 0 };
        const footer = document.getElementById(`node-footer-${n.id}`);
        if (footer) footer.innerHTML = this.renderNodeStatus(n.state);

        // Bỏ preview kết quả cũ trước khi chạy workflow (chỉ giữ ảnh/video tài nguyên đầu vào của input nodes)
        if (!['character_input', 'product_input', 'video_source'].includes(n.type)) {
          const previewEl = document.getElementById(`node-preview-${n.id}`);
          if (previewEl) previewEl.remove();
        }
      });
      this.renderWires();

      this.clearLogs();
      this.appendLog('[Khởi chạy] Đang kiểm tra cấu hình đồ thị Workflow...');

      try {
        const resp = await fetch('/api/flow/execute', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({
            graph: this.graph,
            inputs: workflowInputs,
            muse_credits_acknowledged: providerCreditsAcknowledged,
            google_flow_credits_acknowledged: providerCreditsAcknowledged
          })
        });
        const data = await resp.json();
        if (!data.ok) {
          throw new Error(data.error || 'Lỗi không xác định khi khởi chạy');
        }

        this.currentRunId = data.run_id;
        this.appendLog(`[DAG Engine] Đã kích hoạt phiên chạy ID: ${this.currentRunId}`);
        this.startPollingRun(this.currentRunId);
      } catch (err) {
        if (runBtn) runBtn.disabled = false;
        if (cancelBtn) cancelBtn.style.display = 'none';
        if (statusPill) {
          statusPill.className = 'wf-status-pill status-error';
          statusPill.innerText = '⚠️ Lỗi khởi chạy';
        }
        this.appendLog(`[LỖI] ${err.message}`);
        window.showAppNotice('Lỗi chạy workflow: ' + err.message);
      }
    }

    collectWorkflowRunInputs() {
      const inputNodes = (this.graph.nodes || []).filter(node => node.type === 'workflow_input');
      if (!inputNodes.length) return Promise.resolve({});

      const seen = new Set();
      for (const node of inputNodes) {
        const key = String(node.params?.key || '').trim();
        if (!/^[a-z][a-z0-9_]{0,47}$/.test(key)) {
          return Promise.reject(new Error(`Node “${node.title || key}” cần khóa input hợp lệ`));
        }
        if (seen.has(key)) return Promise.reject(new Error(`Trùng khóa input: ${key}`));
        seen.add(key);
      }

      const modal = document.getElementById('workflow-run-input-modal');
      const form = document.getElementById('workflow-run-input-form');
      const fields = document.getElementById('workflow-run-input-fields');
      const cancel = document.getElementById('btn-cancel-workflow-run-inputs');
      const close = document.getElementById('btn-close-workflow-run-inputs');
      if (!modal || !form || !fields) return Promise.reject(new Error('Thiếu giao diện nhập dữ liệu workflow'));
      const submitButton = form.querySelector('button[type="submit"]');
      const submitButtonLabel = submitButton?.textContent || 'Chạy workflow';
      const updateSubmitUploadState = () => {
        const pending = this.pendingRunInputUploads;
        if (!submitButton) return;
        submitButton.disabled = pending > 0;
        submitButton.textContent = pending > 0 ? `Đang tải ${pending} tệp…` : submitButtonLabel;
      };

      fields.replaceChildren();
      inputNodes.forEach(node => {
        const params = node.params || {};
        const key = String(params.key || '').trim();
        const label = document.createElement('label');
        label.className = 'wf-field-label';
        label.textContent = `${String(params.label || node.title || key)}${params.required === false ? '' : ' *'}`;

        const remembered = Object.prototype.hasOwnProperty.call(this.lastWorkflowInputs, key);
        let value = remembered ? this.lastWorkflowInputs[key] : params.value;
        if (value == null) value = '';
        const type = String(params.value_type || 'text');
        const options = Array.isArray(params.options) ? params.options : [];
        const optionEntries = options.map(option => {
          if (option && typeof option === 'object') {
            return {
              value: String(option.value ?? option.label ?? ''),
              label: String(option.label ?? option.value ?? '')
            };
          }
          return { value: String(option ?? ''), label: String(option ?? '') };
        }).filter(option => option.value !== '');
        const useOptionPicker = optionEntries.length > 0 && ['text', 'any'].includes(type);
        const group = document.createElement('div');
        group.className = 'wf-run-input-field';
        let control;

        if (useOptionPicker) {
          const select = document.createElement('select');
          select.id = `workflow-input-${node.id}`;
          label.htmlFor = select.id;
          select.setAttribute('aria-label', String(params.label || node.title || key));
          const placeholder = document.createElement('option');
          placeholder.value = '';
          placeholder.textContent = 'Chọn một giá trị';
          select.appendChild(placeholder);
          optionEntries.forEach(option => {
            const element = document.createElement('option');
            element.value = option.value;
            element.textContent = option.label;
            select.appendChild(element);
          });
          let customValue = '__workflow_custom__';
          while (optionEntries.some(option => option.value === customValue)) customValue += '_';
          const customOption = document.createElement('option');
          customOption.value = customValue;
          customOption.textContent = 'Tự nhập…';
          select.appendChild(customOption);

          const customInput = document.createElement('input');
          customInput.type = 'text';
          customInput.className = 'wf-form-input';
          customInput.id = `${select.id}-custom`;
          customInput.hidden = true;
          customInput.placeholder = 'Nhập giá trị riêng cho lần chạy này';
          customInput.setAttribute('aria-label', `Giá trị tùy chỉnh: ${String(params.label || node.title || key)}`);

          const valueText = String(value);
          const matchedOption = optionEntries.find(option => option.value === valueText);
          if (matchedOption) {
            select.value = matchedOption.value;
          } else if (valueText !== '') {
            select.value = customValue;
            customInput.value = valueText;
            customInput.hidden = false;
            control = customInput;
          }
          if (!control) control = select;

          select.addEventListener('change', () => {
            if (control.dataset.workflowInputKey) {
              delete control.dataset.workflowInputKey;
              delete control.dataset.workflowInputType;
            }
            const customMode = select.value === customValue;
            customInput.hidden = !customMode;
            label.htmlFor = customMode ? customInput.id : select.id;
            customInput.required = customMode && params.required !== false;
            select.required = !customMode && params.required !== false;
            control = customMode ? customInput : select;
            control.dataset.workflowInputKey = key;
            control.dataset.workflowInputType = type;
            if (customMode) customInput.focus();
          });
          group.append(label, select, customInput);
        } else if (type === 'number') {
          control = document.createElement('input');
          control.type = 'number';
          control.className = 'wf-form-input';
          control.value = value === '' ? '' : String(value);
        } else if (['image', 'image_list', 'video', 'audio'].includes(type)) {
          const row = document.createElement('div');
          row.className = 'wf-run-media-row';
          if (type === 'image_list') {
            control = document.createElement('textarea');
            control.rows = 3;
            control.value = Array.isArray(value) ? value.join('\n') : String(value);
            control.placeholder = 'Một đường dẫn ảnh mỗi dòng';
          } else {
            control = document.createElement('input');
            control.type = 'text';
            control.className = 'wf-form-input';
            control.placeholder = 'Dán đường dẫn hoặc tải tệp';
            control.value = String(value);
          }
          const upload = document.createElement('button');
          upload.type = 'button';
          upload.className = 'wf-btn wf-btn-secondary wf-btn-xs';
          upload.textContent = 'Tải tệp';
          const file = document.createElement('input');
          file.type = 'file';
          file.hidden = true;
          file.accept = type === 'image_list' ? 'image/*' : `${type}/*`;
          file.multiple = type === 'image_list';
          upload.addEventListener('click', () => file.click());
          file.addEventListener('change', async () => {
            if (!file.files?.[0]) return;
            this.pendingRunInputUploads += 1;
            updateSubmitUploadState();
            upload.disabled = true;
            upload.textContent = 'Đang tải…';
            try {
              const paths = [];
              for (const selectedFile of Array.from(file.files || [])) {
                const payload = new FormData();
                payload.append('file', selectedFile);
                const response = await fetch('/api/flow/upload', { method: 'POST', body: payload });
                const result = await response.json();
                if (!result.ok) throw new Error(result.error || 'Tải tệp thất bại');
                paths.push(result.file_path || result.url || '');
              }
              if (type === 'image_list') {
                const existing = control.value.split('\n').map(path => path.trim()).filter(Boolean);
                control.value = [...existing, ...paths.filter(Boolean)].join('\n');
              } else {
                control.value = paths[0] || '';
              }
            } catch (error) {
              window.showAppNotice(error.message || 'Không tải được tệp');
            } finally {
              this.pendingRunInputUploads = Math.max(0, this.pendingRunInputUploads - 1);
              updateSubmitUploadState();
              upload.disabled = false;
              upload.textContent = 'Tải tệp';
            }
          });
          row.append(control, upload, file);
          group.append(label, row);
        } else if (['text_list', 'product', 'motion', 'config'].includes(type)) {
          control = document.createElement('textarea');
          control.className = 'wf-form-textarea';
          control.rows = 4;
          if (type === 'text_list') control.value = Array.isArray(value) ? value.join('\n') : String(value);
          else control.value = value && typeof value === 'object' ? JSON.stringify(value, null, 2) : String(value);
          control.placeholder = type === 'text_list' ? 'Một giá trị mỗi dòng' : 'Nhập dữ liệu JSON';
        } else {
          control = document.createElement('textarea');
          control.className = 'wf-form-textarea';
          control.rows = 3;
          control.value = String(value);
          control.placeholder = params.required === false ? 'Để trống nếu không cần' : 'Nhập giá trị cho lần chạy này';
        }

        control.dataset.workflowInputKey = key;
        control.dataset.workflowInputType = type;
        control.required = params.required !== false;
        if (!['image', 'image_list', 'video', 'audio'].includes(type) && !useOptionPicker) group.append(label, control);
        const helpText = String(params.help || '');
        if (helpText) {
          const help = document.createElement('small');
          help.className = 'wf-run-input-help';
          help.textContent = helpText;
          group.appendChild(help);
        }
        fields.appendChild(group);
      });

      modal.style.display = 'flex';
      const firstControl = fields.querySelector('input:not([type=file]), textarea, select');
      if (firstControl) requestAnimationFrame(() => firstControl.focus());

      return new Promise(resolve => {
        let settled = false;
        const finish = result => {
          if (settled) return;
          settled = true;
          modal.style.display = 'none';
          form.onsubmit = null;
          cancel.onclick = null;
          close.onclick = null;
          modal.onclick = null;
          window.removeEventListener('keydown', onKeyDown);
          resolve(result);
        };
        const onKeyDown = event => { if (event.key === 'Escape') finish(null); };
        form.onsubmit = event => {
          event.preventDefault();
          if (this.pendingRunInputUploads > 0) return;
          if (!form.reportValidity()) return;
          const result = {};
          fields.querySelectorAll('[data-workflow-input-key]').forEach(input => {
            const key = input.dataset.workflowInputKey;
            const type = input.dataset.workflowInputType;
            if (type === 'image_list' || type === 'text_list') {
              result[key] = input.value.split('\n').map(value => value.trim()).filter(Boolean);
            } else if (['product', 'motion', 'config'].includes(type) && input.value.trim()) {
              try {
                result[key] = JSON.parse(input.value);
              } catch (_) {
                input.setCustomValidity('Dữ liệu cần là JSON hợp lệ.');
                input.reportValidity();
                input.setCustomValidity('');
                return;
              }
            } else if (input.type === 'number' && input.value !== '') {
              result[key] = Number(input.value);
            } else {
              result[key] = input.value;
            }
          });
          if (Object.keys(result).length !== inputNodes.length) return;
          this.lastWorkflowInputs = result;
          finish(result);
        };
        cancel.onclick = () => finish(null);
        close.onclick = () => finish(null);
        modal.onclick = event => { if (event.target === modal) finish(null); };
        window.addEventListener('keydown', onKeyDown);
      });
    }

    startPollingRun(runId) {
      if (this.pollInterval) clearInterval(this.pollInterval);

      this.pollInterval = setInterval(async () => {
        try {
          const resp = await fetch(`/api/flow/runs/${runId}`);
          const res = await resp.json();
          if (!res.ok || !res.run) return;

          const run = res.run;
          if (run.outputs) this.renderWorkflowOutputs(run.outputs);
          const progressText = document.getElementById('run-progress-text');
          if (progressText) {
            progressText.innerText = `${run.progress || 0}%`;
          }

          // Update terminal log
          if (run.log) {
            const term = document.getElementById('flow-logs-terminal');
            if (term) {
              term.innerText = run.log;
              term.scrollTop = term.scrollHeight;
            }
          }

          // Update node states
          if (run.node_states) {
            Object.keys(run.node_states).forEach(nodeId => {
              const nodeState = run.node_states[nodeId];
              const node = this.graph.nodes.find(n => n.id === nodeId);
              if (node) {
                const prevStatus = node.state?.status;
                const prevPreview = node.state?.output_preview;
                node.state = nodeState;
                const footer = document.getElementById(`node-footer-${nodeId}`);
                if (footer) footer.innerHTML = this.renderNodeStatus(nodeState, node.type);

                const nodeEl = document.getElementById(`node-${nodeId}`);
                if (nodeEl) {
                  nodeEl.classList.toggle('node-running', nodeState.status === 'running');
                  nodeEl.classList.toggle('node-active-executing', nodeState.status === 'running');
                  nodeEl.classList.toggle('node-completed', nodeState.status === 'completed');
                  nodeEl.classList.toggle('node-failed', nodeState.status === 'failed');
                  nodeEl.classList.toggle('node-cancelled', nodeState.status === 'cancelled');
                }

                // ComfyUI Live Preview: Tự động hiển thị kết quả trực tiếp lên node trên Canvas ngay khi chạy xong
                if (nodeState.output_preview && (nodeState.output_preview !== prevPreview || nodeState.status !== prevStatus)) {
                  this.updateNodeVisualPreview(nodeId, nodeState.output_preview);
                  this.propagatePreviewToOutputs(nodeId, `/media/${this.getFilename(nodeState.output_preview)}`);
                }
              }
            });
            this.renderWires(); // Re-render wires so active running wires light up
          }

          // Terminal completion
          if (run.status === 'completed' || run.status === 'failed' || run.status === 'cancelled') {
            clearInterval(this.pollInterval);
            this.pollInterval = null;

            const runBtn = document.getElementById('btn-run-flow');
            const cancelBtn = document.getElementById('btn-cancel-flow');
            if (runBtn) runBtn.disabled = false;
            if (cancelBtn) cancelBtn.style.display = 'none';

            const statusPill = this.statusPill;
            if (run.status === 'completed') {
              if (statusPill) {
                statusPill.className = 'wf-status-pill status-ready';
                statusPill.innerText = '✓ Hoàn tất 100%';
              }
              this.appendLog(`🎉 [Hoàn tất thành công] Video thành phẩm: ${run.final_output || 'Hoàn tất'}`);
              this.renderWorkflowOutputs(run.outputs || {});
              if (run.final_output) {
                this.showFinalModal(run.final_output);
              }
            } else if (run.status === 'failed') {
              if (statusPill) {
                statusPill.className = 'wf-status-pill status-error';
                statusPill.innerText = '⚠️ Thất bại';
              }
            } else {
              if (statusPill) {
                statusPill.className = 'wf-status-pill status-ready';
                statusPill.innerText = 'Đã dừng';
              }
            }
          }
        } catch (e) {
          console.error('Workflow poll error', e);
        }
      }, 700);
    }

    async cancelFlow() {
      if (!this.currentRunId) return;
      try {
        await fetch(`/api/flow/runs/${this.currentRunId}/cancel`, { method: 'POST' });
        this.appendLog('[Hủy bỏ] Đã gửi tín hiệu dừng tới hệ thống');
      } catch (e) {
        console.error(e);
      }
    }

    showFinalModal(outputUrl) {
      const filename = this.getFilename(outputUrl);
      const isVideo = /\.(mp4|webm|mov)$/i.test(filename);
      const mediaURL = `/media/${encodeURIComponent(filename)}`;

      const modal = document.createElement('div');
      modal.className = 'wf-modal-overlay';
      modal.innerHTML = `
        <div class="wf-modal-window" style="max-width:560px">
          <div class="wf-modal-top">
            <div class="wf-modal-title">
              <span class="wf-modal-title-icon">🎉</span>
              <h3>Video Thành Phẩm Workflow</h3>
            </div>
            <button type="button" class="wf-modal-close">&times;</button>
          </div>
          <div class="wf-final-media" style="padding:20px;text-align:center">
            <div style="margin-top:18px;display:flex;gap:12px;justify-content:center">
              <a class="wf-btn wf-btn-primary wf-final-download" download>⬇ Tải video về máy</a>
              <a href="/studio/jobs" class="wf-btn wf-btn-secondary">Xem danh sách Video ↗</a>
            </div>
          </div>
        </div>
      `;
      const media = document.createElement(isVideo ? 'video' : 'img');
      media.src = mediaURL;
      media.style.maxWidth = '100%';
      media.style.borderRadius = '10px';
      if (isVideo) {
        media.controls = true;
        media.autoplay = true;
        media.style.width = '100%';
        media.style.maxHeight = '480px';
        media.style.background = '#000';
        media.style.boxShadow = '0 8px 24px rgba(0,0,0,0.3)';
      }
      const mediaBox = modal.querySelector('.wf-final-media');
      mediaBox.insertBefore(media, mediaBox.firstChild);
      const download = modal.querySelector('.wf-final-download');
      download.href = mediaURL;
      download.download = filename;
      modal.querySelector('.wf-modal-close').addEventListener('click', () => modal.remove());
      modal.addEventListener('click', (e) => {
        if (e.target === modal) modal.remove();
      });
      document.body.appendChild(modal);
    }

    appendLog(msg) {
      const term = document.getElementById('flow-logs-terminal');
      if (!term) return;
      const line = document.createElement('div');
      line.className = 'wf-log-row';
      line.innerText = msg;
      term.appendChild(line);
      term.scrollTop = term.scrollHeight;
    }

    clearLogs() {
      const term = document.getElementById('flow-logs-terminal');
      if (term) term.innerHTML = '';
    }

    bindInspector() {
      const closeBtn = document.getElementById('btn-close-inspector');
      if (closeBtn) closeBtn.addEventListener('click', () => this.closeInspector());
      document.querySelectorAll('[data-inspector-tab]').forEach(tab => {
        tab.addEventListener('click', () => this.setInspectorTab(tab.dataset.inspectorTab));
      });
    }

    setInspectorTab(tabName) {
      const allowed = new Set(['parameters', 'io', 'result']);
      const activeTab = allowed.has(tabName) ? tabName : 'parameters';
      this.activeInspectorTab = activeTab;
      document.querySelectorAll('[data-inspector-tab]').forEach(tab => {
        const active = tab.dataset.inspectorTab === activeTab;
        tab.classList.toggle('active', active);
        tab.setAttribute('aria-selected', active ? 'true' : 'false');
      });
      document.querySelectorAll('[data-inspector-pane]').forEach(pane => {
        const active = pane.dataset.inspectorPane === activeTab;
        pane.hidden = !active;
        pane.classList.toggle('active', active);
      });
    }

    renderWorkflowBoundaryInspector(node, container) {
      const params = node.params || (node.params = {});
      const addTextField = (key, labelText, value, onChange) => {
        const group = document.createElement('div');
        group.className = 'form-group';
        const label = document.createElement('label');
        label.innerText = labelText;
        const input = document.createElement('input');
        input.type = 'text';
        input.className = 'form-control';
        input.value = value || '';
        input.addEventListener('change', () => onChange(input.value.trim()));
        group.append(label, input);
        container.appendChild(group);
      };

      addTextField('key', 'Khóa input/output', params.key || '', value => {
        params.key = value.toLowerCase().replace(/[^a-z0-9_]/g, '_').replace(/^[^a-z]+/, '');
      });
      addTextField('label', 'Tên hiển thị', params.label || '', value => { params.label = value; });

      const typeGroup = document.createElement('div');
      typeGroup.className = 'form-group';
      const typeLabel = document.createElement('label');
      typeLabel.innerText = 'Kiểu dữ liệu';
      const typeSelect = document.createElement('select');
      typeSelect.className = 'form-control';
      this.getWorkflowPortTypes().forEach(type => {
        const option = document.createElement('option');
        option.value = type;
        option.innerText = type;
        typeSelect.appendChild(option);
      });
      typeSelect.value = this.getWorkflowPortTypes().includes(params.value_type) ? params.value_type : 'any';
      typeSelect.addEventListener('change', () => {
        params.value_type = typeSelect.value;
        this.render();
        this.selectNode(node.id);
      });
      typeGroup.append(typeLabel, typeSelect);
      container.appendChild(typeGroup);

      if (node.type === 'workflow_input') {
        const requiredGroup = document.createElement('label');
        requiredGroup.className = 'form-group';
        const required = document.createElement('input');
        required.type = 'checkbox';
        required.checked = params.required !== false;
        required.addEventListener('change', () => { params.required = required.checked; });
        requiredGroup.append(required, document.createTextNode(' Input bắt buộc'));
        container.appendChild(requiredGroup);

        const optionsGroup = document.createElement('div');
        optionsGroup.className = 'form-group';
        const optionsLabel = document.createElement('label');
        optionsLabel.innerText = 'Lựa chọn nhanh khi chạy';
        const optionsInput = document.createElement('textarea');
        optionsInput.className = 'form-control';
        optionsInput.rows = 3;
        const optionValues = Array.isArray(params.options) ? params.options.map(option => {
          if (option && typeof option === 'object') return String(option.label ?? option.value ?? '');
          return String(option ?? '');
        }) : [];
        optionsInput.value = optionValues.join('\n');
        optionsInput.placeholder = 'Mỗi lựa chọn một dòng. Người chạy vẫn có thể tự nhập giá trị khác.';
        optionsInput.addEventListener('input', () => {
          params.options = optionsInput.value.split('\n').map(value => value.trim()).filter(Boolean);
        });
        const optionsHelp = document.createElement('small');
        optionsHelp.className = 'text-muted';
        optionsHelp.textContent = 'Người chạy có thể chọn một giá trị hoặc chọn “Tự nhập” để đổi cho lần chạy đó.';
        optionsGroup.append(optionsLabel, optionsInput, optionsHelp);
        container.appendChild(optionsGroup);

        const helpGroup = document.createElement('div');
        helpGroup.className = 'form-group';
        const helpLabel = document.createElement('label');
        helpLabel.innerText = 'Hướng dẫn cho người chạy';
        const helpInput = document.createElement('input');
        helpInput.type = 'text';
        helpInput.className = 'form-control';
        helpInput.value = params.help || '';
        helpInput.placeholder = 'Ví dụ: Chọn phong cách phù hợp với sản phẩm';
        helpInput.addEventListener('input', () => { params.help = helpInput.value; });
        helpGroup.append(helpLabel, helpInput);
        container.appendChild(helpGroup);

        const valueGroup = document.createElement('div');
        valueGroup.className = 'form-group';
        const valueLabel = document.createElement('label');
        valueLabel.innerText = 'Giá trị mặc định / đầu vào';
        const valueInput = document.createElement('textarea');
        valueInput.className = 'form-control';
        valueInput.rows = 3;
        valueInput.value = params.value == null ? '' : String(params.value);
        valueInput.placeholder = 'Có thể truyền giá trị này qua webhook bằng khóa input phía trên';
        valueInput.addEventListener('input', () => { params.value = valueInput.value; });
        valueGroup.append(valueLabel, valueInput);
        container.appendChild(valueGroup);

        if (['image', 'image_list', 'video', 'audio'].includes(params.value_type)) {
          const upload = document.createElement('button');
          upload.type = 'button';
          upload.className = 'btn btn-secondary btn-sm';
          upload.innerText = '📁 Tải media làm giá trị mặc định';
          const fileInput = document.createElement('input');
          fileInput.type = 'file';
          fileInput.hidden = true;
          fileInput.accept = params.value_type === 'image' || params.value_type === 'image_list' ? 'image/*' : `${params.value_type}/*`;
          fileInput.addEventListener('change', async () => {
            const file = fileInput.files && fileInput.files[0];
            if (!file) return;
            upload.disabled = true;
            upload.innerText = 'Đang tải…';
            try {
              const form = new FormData();
              form.append('file', file);
              const response = await fetch('/api/flow/upload', { method: 'POST', body: form });
              const result = await response.json();
              if (!result.ok) throw new Error(result.error || 'Tải media thất bại');
              params.value = result.file_path;
              params.preview_url = result.url;
              this.render();
              this.selectNode(node.id);
            } catch (error) {
              window.showAppNotice(error.message || 'Không tải được media');
            } finally {
              upload.disabled = false;
              upload.innerText = '📁 Tải media làm giá trị mặc định';
            }
          });
          upload.addEventListener('click', () => fileInput.click());
          container.append(upload, fileInput);
        }
      }
    }

    getWorkflowContract(graph) {
      const inputs = [];
      const outputs = [];
      const seen = { input: new Set(), output: new Set() };
      const validTypes = new Set(this.getWorkflowPortTypes());
      (graph.nodes || []).forEach(node => {
        if (node.type !== 'workflow_input' && node.type !== 'workflow_output') return;
        const params = node.params || {};
        const key = String(params.key || '').trim();
        if (!/^[a-z][a-z0-9_]{0,47}$/.test(key)) throw new Error(`Node “${node.title || key}” có khóa không hợp lệ`);
        const direction = node.type === 'workflow_input' ? 'input' : 'output';
        if (seen[direction].has(key)) throw new Error(`Trùng khóa ${direction}: ${key}`);
        seen[direction].add(key);
        const type = String(params.value_type || 'any');
        if (!validTypes.has(type)) throw new Error(`Kiểu dữ liệu không hỗ trợ: ${type}`);
        const port = { id: key, name: String(params.label || key), type };
        if (direction === 'input') port.required = params.required !== false;
        (direction === 'input' ? inputs : outputs).push(port);
      });
      if (!inputs.length || !outputs.length) throw new Error('Workflow cần ít nhất một Đầu vào và một Đầu ra');
      return { inputs, outputs };
    }

    renderWorkflowCallInspector(node, container) {
      const params = node.params || (node.params = {});
      const fixedCategory = node.type === 'workflow_call' ? '' : node.type.slice('workflow_call_'.length);
      if (fixedCategory) params.workflow_category = fixedCategory;
      const categories = [
        { value: 'affiliate', label: 'Affiliate' },
        { value: 'viral', label: 'TikTok Viral' },
        { value: 'fashion', label: 'Thời trang' },
        { value: 'custom', label: 'Custom' }
      ];

      const categoryGroup = document.createElement('div');
      categoryGroup.className = 'form-group';
      const categoryLabel = document.createElement('label');
      categoryLabel.innerText = 'Loại workflow';
      if (fixedCategory) {
        const fixed = document.createElement('div');
        fixed.className = 'form-control';
        fixed.innerText = (categories.find(c => c.value === fixedCategory) || { label: fixedCategory }).label;
        categoryGroup.append(categoryLabel, fixed);
      } else {
        const categorySelect = document.createElement('select');
        categorySelect.className = 'form-control';
        const any = document.createElement('option');
        any.value = '';
        any.innerText = 'Tất cả loại';
        categorySelect.appendChild(any);
        categories.forEach(category => {
          const option = document.createElement('option');
          option.value = category.value;
          option.innerText = category.label;
          categorySelect.appendChild(option);
        });
        categorySelect.value = params.workflow_category || '';
        categorySelect.addEventListener('change', async () => {
          const connected = this.graph.edges.some(edge => edge.from_node === node.id || edge.to_node === node.id);
          if (connected && !await window.showAppConfirm('Đổi loại workflow sẽ xóa các kết nối hiện tại của node. Tiếp tục?', { title: 'Đổi loại workflow', confirmText: 'Đổi loại', danger: true })) {
            categorySelect.value = params.workflow_category || '';
            return;
          }
          this.workflowContractRequests.set(node.id, Symbol('category-changed'));
          this.workflowContractLoading.delete(node.id);
          params.workflow_category = categorySelect.value;
          params.workflow_id = '';
          this.graph.edges = this.graph.edges.filter(edge => edge.from_node !== node.id && edge.to_node !== node.id);
          node.inputs = [];
          node.outputs = [];
          this.render();
          this.selectNode(node.id);
        });
        categoryGroup.append(categoryLabel, categorySelect);
      }
      container.appendChild(categoryGroup);

      const flowGroup = document.createElement('div');
      flowGroup.className = 'form-group';
      const flowLabel = document.createElement('label');
      flowLabel.innerText = 'Workflow đã lưu';
      const flowSelect = document.createElement('select');
      flowSelect.className = 'form-control';
      const placeholder = document.createElement('option');
      placeholder.value = '';
      placeholder.innerText = 'Chọn workflow đã sẵn sàng…';
      flowSelect.appendChild(placeholder);
      const candidates = this.savedGraphs.filter(graph => graph.id !== this.graph.id && (!params.workflow_category || graph.category === params.workflow_category));
      const unusableCandidates = candidates.filter(graph => graph.workflow_reusable === false);
      candidates.forEach(graph => {
        const option = document.createElement('option');
        option.value = graph.id;
        option.disabled = graph.workflow_reusable === false;
        option.innerText = `${graph.name} · ${graph.category || 'custom'}${option.disabled ? ' · chưa sẵn sàng' : ''}`;
        if (option.disabled) option.title = graph.workflow_error || 'Kiểm tra workflow và khai báo Đầu vào/Đầu ra.';
        flowSelect.appendChild(option);
      });
      flowSelect.value = params.workflow_id || '';
      flowSelect.addEventListener('change', async () => {
        if (!flowSelect.value) {
          const connected = this.graph.edges.some(edge => edge.from_node === node.id || edge.to_node === node.id);
          if (connected && !await window.showAppConfirm('Bỏ workflow con sẽ xóa các kết nối của node. Tiếp tục?', { title: 'Bỏ workflow con', confirmText: 'Bỏ workflow', danger: true })) {
            flowSelect.value = params.workflow_id || '';
            return;
          }
          this.workflowContractRequests.set(node.id, Symbol('contract-cleared'));
          this.workflowContractLoading.delete(node.id);
          params.workflow_id = '';
          params.workflow_name = '';
          node.inputs = [];
          node.outputs = [];
          this.graph.edges = this.graph.edges.filter(edge => edge.from_node !== node.id && edge.to_node !== node.id);
          this.render();
          this.selectNode(node.id);
          return;
        }
        this.loadWorkflowContract(node, flowSelect.value);
      });
      flowGroup.append(flowLabel, flowSelect);
      container.appendChild(flowGroup);
      const prerequisiteHint = document.createElement('small');
      prerequisiteHint.className = 'text-muted';
      prerequisiteHint.innerText = 'Workflow con phải được lưu, có node Đầu vào và Đầu ra, đồng thời qua kiểm tra cấu hình.';
      flowGroup.appendChild(prerequisiteHint);
      if (!candidates.length) {
        const hint = document.createElement('small');
        hint.className = 'text-muted';
        hint.innerText = params.workflow_category
          ? `Chưa có workflow đã lưu thuộc loại ${params.workflow_category}.`
          : 'Chưa có workflow khác để gọi.';
        flowGroup.appendChild(hint);
      }
      if (unusableCandidates.length) {
        const details = document.createElement('details');
        details.style.marginTop = '6px';
        const summary = document.createElement('summary');
        summary.innerText = `${unusableCandidates.length} workflow chưa sẵn sàng`;
        details.appendChild(summary);
        const reasons = document.createElement('ul');
        reasons.style.cssText = 'margin:4px 0 0;padding-left:20px';
        unusableCandidates.forEach(graph => {
          const item = document.createElement('li');
          item.innerText = `${graph.name}: ${graph.workflow_error || 'kiểm tra node Đầu vào và Đầu ra'}`;
          reasons.appendChild(item);
        });
        details.appendChild(reasons);
        flowGroup.appendChild(details);
      }

      if (params.workflow_id && (!node.inputs?.length || !node.outputs?.length) && !this.workflowContractLoading.has(node.id)) {
        this.loadWorkflowContract(node, params.workflow_id);
      }
      if (params.workflow_id) {
        const refresh = document.createElement('button');
        refresh.type = 'button';
        refresh.className = 'btn btn-secondary btn-sm';
        refresh.innerText = 'Đồng bộ cổng từ workflow con';
        refresh.title = 'Dùng sau khi sửa input/output workflow con và lưu workflow đó.';
        refresh.addEventListener('click', () => this.loadWorkflowContract(node, params.workflow_id));
        container.appendChild(refresh);
        const refreshHint = document.createElement('small');
        refreshHint.className = 'text-muted';
        refreshHint.innerText = 'Sau khi đổi contract workflow con, đồng bộ lại cổng. Kết nối không tương thích sẽ được liệt kê trước khi xóa.';
        container.appendChild(refreshHint);
      }
    }

    renderWorkflowMapInspector(node, container) {
      const params = node.params || (node.params = {});
      const candidates = this.savedGraphs.filter(graph => graph.id !== this.graph.id);
      const makeGroup = (title, current, options, placeholderText, onChange) => {
        const group = document.createElement('div');
        group.className = 'form-group';
        const label = document.createElement('label');
        label.textContent = title;
        const select = document.createElement('select');
        select.className = 'form-control';
        const placeholder = document.createElement('option');
        placeholder.value = '';
        placeholder.textContent = placeholderText;
        select.appendChild(placeholder);
        options.forEach(optionData => {
          const option = document.createElement('option');
          option.value = optionData.value;
          option.textContent = optionData.label;
          option.disabled = Boolean(optionData.disabled);
          if (option.disabled) option.title = optionData.title || '';
          select.appendChild(option);
        });
        select.value = current || '';
        select.addEventListener('change', () => onChange(select.value));
        group.append(label, select);
        container.appendChild(group);
        return select;
      };

      const workflows = candidates.map(graph => {
        const inputs = Array.isArray(graph.inputs) ? graph.inputs : [];
        const outputs = Array.isArray(graph.outputs) ? graph.outputs : [];
        const disabled = graph.workflow_reusable === false || !inputs.length || !outputs.length;
        return {
          value: graph.id,
          label: `${graph.name} · ${inputs.length} input / ${outputs.length} output${disabled ? ' · chưa sẵn sàng' : ''}`,
          disabled,
          title: graph.workflow_error || 'Workflow cần ít nhất một input và một output hợp lệ.'
        };
      });

      makeGroup('Workflow con cho mỗi phần tử', params.workflow_id, workflows, 'Chọn workflow đã lưu…', workflowID => {
        const graph = candidates.find(item => item.id === workflowID);
        params.workflow_id = workflowID;
        params.workflow_name = graph?.name || '';
        params.item_input_key = graph?.inputs?.[0]?.id || '';
        params.index_input_key = '';
        params.result_output_key = graph?.outputs?.[0]?.id || '';
        this.render();
        this.selectNode(node.id);
      });

      const selected = candidates.find(graph => graph.id === params.workflow_id);
      if (!selected) {
        const help = document.createElement('small');
        help.className = 'text-muted';
        help.textContent = 'Workflow con cần input/output contract hợp lệ. Mỗi phần tử chạy thành một lần thực thi riêng, giữ nguyên thứ tự.';
        container.appendChild(help);
        return;
      }

      const inputs = Array.isArray(selected.inputs) ? selected.inputs : [];
      const outputs = Array.isArray(selected.outputs) ? selected.outputs : [];
      makeGroup('Input nhận từng phần tử', params.item_input_key, inputs.map(port => ({
        value: port.id,
        label: `${port.name || port.id} · ${port.type}`
      })), 'Chọn input…', value => { params.item_input_key = value; });
      makeGroup('Input thứ tự (tùy chọn)', params.index_input_key, inputs.filter(port => port.type === 'number').map(port => ({
        value: port.id,
        label: `${port.name || port.id} · number`
      })), 'Không truyền số thứ tự', value => { params.index_input_key = value; });
      makeGroup('Output thu về cho mỗi phần tử', params.result_output_key, outputs.map(port => ({
        value: port.id,
        label: `${port.name || port.id} · ${port.type}`
      })), 'Chọn output…', value => { params.result_output_key = value; });

      const summary = document.createElement('small');
      summary.className = 'text-muted';
      summary.textContent = 'Input bắt buộc khác cần có giá trị mặc định trong workflow con. Giới hạn xử lý đặt ở phần tham số bên dưới.';
      container.appendChild(summary);
    }

    async loadWorkflowContract(node, workflowId, rerender = true) {
      const requestToken = Symbol('workflow-contract');
      const previousWorkflowId = node.params.workflow_id || '';
      this.workflowContractRequests.set(node.id, requestToken);
      this.workflowContractLoading.add(node.id);
      try {
        const response = await fetch(`/api/flow/graphs/${encodeURIComponent(workflowId)}`);
        const result = await response.json();
        if (!response.ok || !result.ok || !result.graph) throw new Error(result.error || 'Không tải được workflow');
        if (this.workflowContractRequests.get(node.id) !== requestToken) return;
        if (result.workflow_reusable === false) throw new Error(result.workflow_error || 'Workflow cần input/output hợp lệ và phải qua kiểm tra trước khi gọi');
        const contract = this.getWorkflowContract(result.graph);
        const category = node.params.workflow_category || '';
        if (category && result.graph.category !== category) throw new Error(`Workflow thuộc loại ${result.graph.category}, cần ${category}`);
        const oldInputSignature = JSON.stringify(node.inputs || []);
        const oldOutputSignature = JSON.stringify(node.outputs || []);
        const nextInputs = new Map(contract.inputs.map(port => [port.id, port]));
        const nextOutputs = new Map(contract.outputs.map(port => [port.id, port]));
        const compatibleEdges = [];
        const removedEdges = [];
        this.graph.edges.forEach(edge => {
          let compatible = true;
          if (edge.to_node === node.id) {
            const targetPort = nextInputs.get(edge.to_port);
            const sourceNode = this.graph.nodes.find(candidate => candidate.id === edge.from_node);
            const sourcePort = sourceNode && this.getNodePorts(sourceNode, true).find(port => port.id === edge.from_port);
            compatible = Boolean(targetPort && sourcePort && this.isPortTypeCompatible(sourcePort.type, targetPort.type));
          } else if (edge.from_node === node.id) {
            const sourcePort = nextOutputs.get(edge.from_port);
            const targetNode = this.graph.nodes.find(candidate => candidate.id === edge.to_node);
            const targetPort = targetNode && this.getNodePorts(targetNode, false).find(port => port.id === edge.to_port);
            compatible = Boolean(sourcePort && targetPort && this.isPortTypeCompatible(sourcePort.type, targetPort.type));
          }
          (compatible ? compatibleEdges : removedEdges).push(edge);
        });
        if (removedEdges.length && !await window.showAppConfirm(`Contract của “${result.graph.name}” không còn tương thích với ${removedEdges.length} kết nối. Đồng bộ sẽ xóa các kết nối đó. Tiếp tục?`, { title: 'Đồng bộ workflow con', confirmText: 'Đồng bộ và xóa dây', danger: true })) {
          if (rerender) {
            this.render();
            this.selectNode(node.id);
          }
          return;
        }
        this.graph.edges = compatibleEdges;
        node.inputs = contract.inputs;
        node.outputs = contract.outputs;
        node.params.workflow_id = workflowId;
        node.params.workflow_name = result.graph.name;
        const contractChanged = oldInputSignature !== JSON.stringify(contract.inputs) || oldOutputSignature !== JSON.stringify(contract.outputs);
        if (removedEdges.length) {
          window.showAppNotice(`Đã đồng bộ contract và xóa ${removedEdges.length} kết nối không tương thích. Kiểm tra canvas rồi lưu workflow.`);
        } else if (contractChanged) {
          window.showAppNotice('Đã đồng bộ contract workflow con. Kiểm tra các cổng rồi lưu workflow.');
        }
        if (rerender) {
          this.render();
          this.selectNode(node.id);
        }
      } catch (error) {
        if (this.workflowContractRequests.get(node.id) !== requestToken) return;
        node.params.workflow_id = previousWorkflowId;
        if (rerender) {
          this.render();
          this.selectNode(node.id);
        }
        window.showAppNotice(error.message || 'Không tải được contract workflow');
      } finally {
        if (this.workflowContractRequests.get(node.id) === requestToken) {
          this.workflowContractRequests.delete(node.id);
          this.workflowContractLoading.delete(node.id);
        }
      }
    }

    parameterChoices(key, node) {
      const choices = {
        voice: [
          ['warm-female', 'Nữ · ấm áp, kể chuyện'],
          ['clear-teacher', 'Nữ · rõ ràng'],
          ['upbeat-host', 'Nữ · tươi sáng'],
          ['energetic-caster', 'Nam · năng lượng'],
          ['male', 'Nam · trầm'],
          ['default', 'Mặc định hệ thống']
        ],
        provider: [['googleflow', 'Google Flow (Veo)'], ['museai', 'Muse AI']],
        provider_name: [['googleflow', 'Google Flow (Veo)'], ['museai', 'Muse AI']],
        aspect: node.type === 'google_flow_video'
          ? [['VIDEO_ASPECT_RATIO_PORTRAIT', '9:16 · Dọc'], ['VIDEO_ASPECT_RATIO_LANDSCAPE', '16:9 · Ngang']]
          : [['9:16', '9:16 · Dọc (TikTok/Reels/Shorts)'], ['16:9', '16:9 · Ngang (YouTube/TV)'], ['1:1', '1:1 · Vuông (Feed)']],
        operation: [['resize_aspect', 'Đổi tỷ lệ khung hình'], ['color_grade', 'Cân màu điện ảnh']],
        aspect_ratio: [['9:16', '9:16 · Dọc'], ['16:9', '16:9 · Ngang'], ['1:1', '1:1 · Vuông'], ['4:5', '4:5 · Dọc']],
        filter_preset: [['cinematic_vibrant', 'Điện ảnh · màu rực'], ['cyber_neon', 'Cyber neon'], ['vintage_warm', 'Vintage ấm']],
        fabric_type: [
          ['silk_satin', 'Lụa Satin cao cấp (Mềm mịn, óng ả, rủ sóng nước)'],
          ['cotton_twill', 'Cotton Twill (Dệt chéo đứng phom chuẩn)'],
          ['chiffon_sheer', 'Chiffon mỏng nhẹ (Bay bổng, mềm mại)'],
          ['leather_gloss', 'Da bóng thời trang (Glossy Leather bắt sáng)'],
          ['denim_heavy', 'Vải Jean Denim (Dày dặn, cá tính)'],
          ['velvet_plush', 'Nhung tuyết xa xỉ (Plush Velvet sang trọng)'],
          ['linen_breeze', 'Linen tự nhiên (Thoáng mát, mộc mạc)']
        ],
        cloth_tension: [
          ['fluid_drape', 'Rủ tự nhiên theo đường cong cơ thể (Fluid Drape)'],
          ['liquid_flow', 'Chuyển động sóng nước mềm mại (Liquid Flow)'],
          ['stiff_structured', 'Giữ phom cứng cáp định hình (Structured)'],
          ['wind_flutter', 'Bay tà bồng bềnh trong gió (Wind Flutter)']
        ],
        drape_flow: [
          ['liquid_flow', 'Chuyển động sóng nước mềm mại (Liquid Flow)'],
          ['fluid_drape', 'Rủ tự nhiên mềm mại (Fluid Drape)'],
          ['stiff_structured', 'Giữ phom cứng cáp (Structured)'],
          ['wind_flutter', 'Bay tà bồng bềnh trong gió (Wind Flutter)']
        ],
        simulation_quality: [
          ['ultra_high', 'Siêu cao cấp 4K (Ultra High Mesh)'],
          ['high', 'Chất lượng cao (High Quality)'],
          ['balanced', 'Cân bằng tốc độ & chất lượng (Balanced)']
        ],
        lut_profile: [
          ['editorial_vogue_2026', 'Editorial Vogue 2026 (Chuẩn tạp chí thời trang)'],
          ['cinematic_teal_orange', 'Điện ảnh Teal & Orange'],
          ['warm_film_35mm', 'Film nhựa 35mm ấm áp cổ điển'],
          ['clean_commercial_pop', 'Thương mại trong trẻo tươi tắn'],
          ['cyber_neon_glow', 'Cyber Neon sắc sảo hiện đại']
        ],
        shot_style: [
          ['high_fashion_editorial', 'Thời trang High-Fashion điện ảnh'],
          ['dynamic_dance_tracking', 'Bắt nét chuyển động vũ đạo mượt mà'],
          ['commercial_luxury', 'TVC thương mại sang trọng'],
          ['tiktok_viral_punchy', 'TikTok Viral cận cảnh chuyển động']
        ],
        dance_style: [
          ['sexy_dance_tiktok', 'Sexy Dance quyến rũ TikTok'],
          ['kpop_girlgroup_sharp', 'Vũ đạo K-pop Girlgroup sắc nét'],
          ['hiphop_street_flow', 'Hip-hop đường phố mạnh mẽ'],
          ['waacking_voguing', 'Waacking / Voguing thời trang']
        ],
        dance_type: [
          ['sexy_dance', 'Sexy Dance quyến rũ gợi cảm'],
          ['kpop_dance', 'K-pop Dance đồng đều bắt mắt'],
          ['modern_dance', 'Nhảy hiện đại & đương đại'],
          ['street_dance', 'Nhảy đường phố sôi động']
        ],
        motion_fidelity: [
          ['exact_100', 'Khớp chính xác 100% động tác gốc'],
          ['smooth_enhanced', 'Khớp 95% + Tăng độ mượt mà chuyển động'],
          ['stylized_dance', 'Khớp động tác + Phong cách hóa theo dáng người']
        ],
        pose_style: [
          ['catwalk_runway', 'Catwalk sải bước sàn diễn thời trang'],
          ['fashion_pose_editorial', 'Tạo dáng Lookbook thời trang tĩnh'],
          ['dynamic_rhythm', 'Nhún nhảy uyển chuyển theo điệu nhạc']
        ],
        skeleton_precision: [
          ['high_density', 'Mật độ cao DensePose (Chính xác từng khớp)'],
          ['standard_pose', 'Chuẩn MediaPipe / OpenPose']
        ],
        bitrate_mode: [
          ['high_quality', 'Bitrate chất lượng cao (Không nén)'],
          ['cbr_tiktok_optimized', 'Tối ưu thuật toán nén TikTok (1080p 60fps)']
        ],
        target_resolution: [
          ['1080x1920', '1080x1920 (TikTok 9:16 Full HD)'],
          ['2160x3840', '2160x3840 (TikTok 9:16 4K Ultra HD)'],
          ['1920x1080', '1920x1080 (YouTube 16:9 Ngang Full HD)'],
          ['1080x1080', '1080x1080 (Instagram 1:1 Vuông)']
        ],
        key_position: [
          ['front_left_45', 'Đèn chính góc 45° bên trái'],
          ['front_right_45', 'Đèn chính góc 45° bên phải'],
          ['top_overhead_beauty', 'Đèn trên đỉnh tạo bóng mắt sang trọng (Beauty Light)']
        ],
        fill_ratio: [
          ['1:2', 'Tỉ lệ 1:2 (Ánh sáng studio mềm mại tự nhiên)'],
          ['1:3', 'Tỉ lệ 1:3 (Đậm chất điện ảnh, giàu chiều sâu)'],
          ['1:1', 'Tỉ lệ 1:1 (Sáng đều phong cách Lookbook)']
        ],
        mood: [
          ['high_fashion_studio', 'Studio thời trang xa xỉ'],
          ['glamour_neon_stage', 'Sân khấu đèn màu Glamour'],
          ['warm_natural_sun', 'Nắng tự nhiên ấm áp']
        ],
        aperture: [
          ['f/1.4', 'f/1.4 (Xóa phông mờ mịt, nổi bật chủ thể)'],
          ['f/1.8', 'f/1.8 (Xóa phông điện ảnh tiêu chuẩn)'],
          ['f/2.8', 'f/2.8 (Rõ nét cả chủ thể & chi tiết bối cảnh)']
        ],
        focal_length: [
          ['35mm', '35mm (Góc rộng thời trang, kéo dài chân tôn dáng)'],
          ['50mm', '50mm (Góc mắt người chân thực)'],
          ['85mm', '85mm (Chân dung xóa phông nén cảnh)']
        ],
        camera_motion: [
          ['fixed_tripod', 'Chân máy cố định chống rung'],
          ['smooth_push_in', 'Đẩy máy quay từ từ lại gần (Push-in)'],
          ['orbit_slow', 'Quay vòng quanh nhân vật 360°'],
          ['dynamic_handheld', 'Cầm tay nhẹ nhàng tự nhiên']
        ],
        rendering_engine: [
          ['muse_chat_mcp', 'Meta Muse AI Engine (Neural Diffusion)'],
          ['google_flow_veo', 'Google Flow Veo 3.1'],
          ['wan_video_2026', 'Wan Video 2.1 / 2026']
        ],
        motion_smoothness: [
          ['fluid_cinema', 'Chuyển động điện ảnh mượt mà 60fps'],
          ['standard_motion', 'Chuyển động chuẩn tự nhiên 30fps'],
          ['dynamic_impact', 'Chuyển động dứt khoát theo beat nhạc']
        ],
        gender: [
          ['female', 'Nữ (Female Avatar)'],
          ['male', 'Nam (Male Avatar)'],
          ['unisex', 'Phi giới tính (Unisex)']
        ],
        expression: [
          ['sensual_alluring', 'Quyến rũ, gợi cảm và cuốn hút'],
          ['confident_smile', 'Nụ cười tự tin rạng rỡ'],
          ['cool_fierce', 'Thần thái sắc lạnh cá tính'],
          ['playful_wink', 'Nháy mắt tinh nghịch vui tươi']
        ],
        placement: [
          ['holding_hand', 'Nhân vật cầm trên tay'],
          ['wearing_body', 'Mặc trên người'],
          ['beside_character', 'Đặt cạnh nhân vật'],
          ['floating_magic', 'Lơ lửng hiệu ứng ma thuật']
        ],
        fit_style: [
          ['seamless_luxury', 'May đo cao cấp ôm dáng (Seamless Luxury)'],
          ['sexy_dance_fit', 'Trang phục nhảy K-pop quyến rũ'],
          ['kpop_stage_outfit', 'Trang phục sân khấu K-pop'],
          ['bodycon_glamour', 'Bodycon tôn đường cong sang trọng']
        ],
        location_type: [
          ['luxury_penthouse', 'Penthouse cao cấp'],
          ['minimalist_studio', 'Studio tối giản'],
          ['fashion_boutique', 'Boutique thời trang Pháp'],
          ['paris_runway', 'Sàn diễn Paris Runway'],
          ['cafe_chill', 'Quán cà phê Hàn Quốc'],
          ['sunset_rooftop', 'Rooftop hoàng hôn lộng gió'],
          ['neon_dance_studio', 'Studio nhảy đèn LED Neon'],
          ['kpop_concert_stage', 'Sân khấu K-pop hoành tráng'],
          ['cyberpunk_rooftop', 'Rooftop cyberpunk mưa đêm'],
          ['sensual_club_lounge', 'Club lounge ánh sáng quyến rũ'],
          ['custom', 'Tùy chỉnh theo prompt']
        ],
        time_of_day: [
          ['golden_hour', 'Hoàng hôn Golden Hour vàng ấm'],
          ['morning_sunlight', 'Nắng ban mai trong trẻo'],
          ['high_noon_editorial', 'Ánh sáng thẳng sắc sảo Editorial'],
          ['blue_hour_dusk', 'Chạng vạng Blue Hour huyền ảo'],
          ['night_neon', 'Neon ban đêm rực rỡ'],
          ['dynamic_stage_lights', 'Đèn sân khấu chuyển động theo nhạc'],
          ['studio_three_point', 'Đèn studio 3 điểm tiêu chuẩn'],
          ['midnight_luxury', 'Đêm sang trọng huyền bí']
        ],
        color_temperature: [
          ['warm_golden', 'Vàng ấm áp (3200K)'],
          ['neutral_daylight', 'Ánh sáng tự nhiên (5600K)'],
          ['cool_blue', 'Xanh lạnh huyền bí (7000K)']
        ],
        light_intensity: [
          ['soft_diffused', 'Khuếch tán mềm mịn tôn da (Soft Diffused)'],
          ['dramatic_high_contrast', 'Tương phản gắt ấn tượng (Dramatic)'],
          ['glamour_glow', 'Tỏa sáng rực rỡ (Glamour Glow)']
        ],
        resolution: [['1080p', '1080p (Full HD)'], ['4k', '4K (Ultra HD)']],
        fit_mode: [['contain', 'Giữ trọn khung hình · có thể có viền'], ['cover', 'Lấp đầy khung hình · cắt phần thừa']],
        audio_codec: [['aac', 'AAC (Chuẩn TikTok)'], ['copy', 'Giữ codec gốc']],
        method: [['POST', 'POST'], ['GET', 'GET'], ['PUT', 'PUT'], ['PATCH', 'PATCH'], ['DELETE', 'DELETE'], ['HEAD', 'HEAD']]
      };
      if (key === 'output_type') {
        const allowed = node.type === 'custom_universal_ai'
          ? [['auto', 'Tự chọn (mặc định là văn bản)'], ['text', 'Văn bản'], ['image', 'Ảnh'], ['video', 'Video']]
          : [['text', 'Văn bản'], ['image', 'Ảnh'], ['video', 'Video']];
        return allowed;
      }
      return choices[key] || null;
    }

    createSliderOrNumberInput(key, val, node, id, onChange) {
      const sliderConfigs = {
        voice_volume: { min: 0, max: 2, step: 0.05, unit: 'x' },
        music_volume: { min: 0, max: 2, step: 0.05, unit: 'x' },
        wrinkle_depth: { min: 0, max: 1, step: 0.05, unit: '' },
        stiffness: { min: 0, max: 1, step: 0.05, unit: '' },
        blend_ratio: { min: 0.5, max: 1, step: 0.02, unit: '' },
        face_similarity_threshold: { min: 0.5, max: 1, step: 0.02, unit: '' },
        contrast_boost: { min: 0.8, max: 1.8, step: 0.02, unit: 'x' },
        saturation: { min: 0.5, max: 2.0, step: 0.05, unit: 'x' },
        gravity_factor: { min: 0, max: 20, step: 0.5, unit: ' m/s²' },
        wind_force: { min: 0, max: 10, step: 0.2, unit: ' m/s' },
        key_light_lux: { min: 200, max: 5000, step: 100, unit: ' Lux' },
        rim_color_kelvin: { min: 2500, max: 9000, step: 100, unit: 'K' },
        scale: { min: 0.1, max: 2.0, step: 0.05, unit: 'x' },
        fps: { min: 12, max: 60, step: 1, unit: ' fps' }
      };
      const cfg = sliderConfigs[key];
      if (!cfg) return null;

      const numVal = typeof val === 'number' ? val : (parseFloat(val) || cfg.min);
      const wrap = document.createElement('div');
      wrap.className = 'wf-slider-control';
      wrap.style.cssText = 'display:flex;align-items:center;gap:10px;width:100%;margin-top:4px;';

      const range = document.createElement('input');
      range.type = 'range';
      range.min = cfg.min;
      range.max = cfg.max;
      range.step = cfg.step;
      range.value = numVal;
      range.style.cssText = 'flex:1;cursor:pointer;accent-color:var(--primary,#6366f1);height:6px;';

      const numInput = document.createElement('input');
      numInput.type = 'number';
      numInput.className = 'form-control';
      numInput.min = cfg.min;
      numInput.max = cfg.max;
      numInput.step = cfg.step;
      numInput.value = numVal;
      numInput.style.cssText = 'width:78px;text-align:right;padding:4px 8px;font-size:12px;font-weight:600;border-radius:6px;';
      if (id) numInput.id = id;

      const badge = document.createElement('span');
      badge.style.cssText = 'font-size:11px;color:var(--muted,#94a3b8);min-width:30px;white-space:nowrap;';
      badge.textContent = cfg.unit || '';

      const handleUpdate = (newVal) => {
        const v = parseFloat(newVal);
        const valid = isNaN(v) ? cfg.min : v;
        range.value = valid;
        numInput.value = valid;
        if (typeof onChange === 'function') onChange(valid);
      };

      range.addEventListener('input', () => handleUpdate(range.value));
      numInput.addEventListener('input', () => handleUpdate(numInput.value));

      wrap.appendChild(range);
      wrap.appendChild(numInput);
      if (cfg.unit) wrap.appendChild(badge);
      return wrap;
    }

    isMediaParameter(key) {
      if (['affiliate_url', 'webhook_url', 'product_url'].includes(key)) return false;
      return /(?:^|_)(?:image|video|audio|media)(?:_|$)/i.test(key) || /(?:^|_)path$/i.test(key);
    }

    clearVideoSourceOtherParam(node, selectedKey) {
      if (node.type !== 'video_source') return;
      const otherKey = selectedKey === 'video_url' ? 'video_path' : 'video_url';
      if (!node.params) node.params = {};
      node.params[otherKey] = '';
      [`inspector-field-${otherKey}`].forEach(fieldId => {
        const field = document.getElementById(fieldId);
        if (field) field.value = '';
      });
    }

    preferredMediaType(key) {
      if (key.includes('audio')) return 'audio';
      if (key.includes('video')) return 'video';
      if (key.includes('image')) return 'image';
      return 'auto';
    }

    createChoiceSelect(key, value, node, id) {
      const choices = this.parameterChoices(key, node);
      if (!choices) return null;
      const select = document.createElement('select');
      select.className = 'form-control';
      if (id) select.id = id;
      choices.forEach(([choiceValue, label]) => {
        const option = document.createElement('option');
        option.value = choiceValue;
        option.textContent = label;
        select.appendChild(option);
      });
      const current = String(value ?? '');
      if (current && !choices.some(([choiceValue]) => choiceValue === current)) {
        const option = document.createElement('option');
        option.value = current;
        option.textContent = `Giá trị đang lưu: ${current}`;
        select.appendChild(option);
      }
      select.value = current || choices[0][0];
      return select;
    }

    createProductSelect(node, currentValue, id) {
      const select = document.createElement('select');
      select.className = 'form-control';
      if (id) select.id = id;
      const placeholder = document.createElement('option');
      placeholder.value = '';
      placeholder.textContent = 'Chọn sản phẩm đã duyệt…';
      select.appendChild(placeholder);
      const current = String(currentValue || '');
      if (current) {
        const option = document.createElement('option');
        option.value = current;
        option.textContent = `Sản phẩm #${current} · đang tải…`;
        select.appendChild(option);
        select.value = current;
      }
      select.addEventListener('change', () => {
        if (!node.params) node.params = {};
        node.params.product_id = select.value;
        const product = (select._flowProducts || []).find(item => String(item.id) === select.value);
        if (!product) return;
        node.params.product_title = product.title;
        node.params.product_image = '';
        node.params.preview_url = product.image_url || '';
        ['inspector-field-product_title'].forEach(fieldId => {
          const field = document.getElementById(fieldId);
          if (field) field.value = product.title;
        });
        ['inspector-field-product_image'].forEach(fieldId => {
          const field = document.getElementById(fieldId);
          if (field) field.value = '';
        });
        if (product.image_url) this.updateNodeVisualPreview(node.id, product.image_url);
      });
      fetch('/api/flow/products', { headers: { Accept: 'application/json' } })
        .then(response => response.json().then(data => ({ response, data })))
        .then(({ response, data }) => {
          if (!response.ok || !data.ok) throw new Error(data.error || 'Không tải được catalog');
          select._flowProducts = data.products || [];
          (data.products || []).forEach(product => {
            const option = document.createElement('option');
            option.value = String(product.id);
            option.textContent = `${product.title}${product.category ? ` · ${product.category}` : ''}`;
            select.appendChild(option);
          });
          if (current && !(data.products || []).some(product => String(product.id) === current)) {
            const stale = Array.from(select.options).find(option => option.value === current);
            if (stale) stale.textContent = `Sản phẩm #${current} · không còn đủ điều kiện`;
          }
          select.value = current;
          if (!(data.products || []).length) placeholder.textContent = 'Catalog chưa có sản phẩm được duyệt';
        })
        .catch(() => {
          placeholder.textContent = 'Không tải được catalog sản phẩm';
        });
      return select;
    }

    appendRadarPicker(group, node, input) {
      const topicSelect = document.createElement('select');
      topicSelect.className = 'form-control';
      const topicPlaceholder = document.createElement('option');
      topicPlaceholder.value = '';
      topicPlaceholder.textContent = 'Chủ đề Radar…';
      topicSelect.appendChild(topicPlaceholder);

      const select = document.createElement('select');
      select.className = 'form-control';
      const placeholder = document.createElement('option');
      placeholder.value = '';
      placeholder.textContent = 'Chọn video từ Radar…';
      select.appendChild(placeholder);
      const current = String(node.params?.video_url || '');
      select.value = current;
      const hint = document.createElement('small');
      hint.className = 'text-muted';
      hint.innerHTML = 'Video chưa được xác thực sẽ không chạy ở workflow yêu cầu vũ đạo. <a href="/studio/trends" target="_blank" rel="noopener">Mở Radar để xem và xác nhận</a>.';
      select.addEventListener('change', () => {
        if (!select.value) return;
        if (!node.params) node.params = {};
        node.params.video_url = select.value;
        this.clearVideoSourceOtherParam(node, 'video_url');
        if (input) input.value = select.value;
      });
      group.append(topicSelect, select, hint);
      let requestVersion = 0;
      const loadTopic = async topic => {
        const version = ++requestVersion;
        placeholder.textContent = 'Đang tải video Radar…';
        select.replaceChildren(placeholder);
        try {
          const url = topic ? `/studio/trends?topic=${encodeURIComponent(topic)}` : '/studio/trends';
          const response = await fetch(url, { headers: { Accept: 'application/json' } });
          const data = await response.json();
          if (version !== requestVersion) return;
          if (!response.ok || (data.videos != null && !Array.isArray(data.videos))) throw new Error(data.error || 'Không tải được Radar');
          const topics = data.topics || [];
          if (topicSelect.options.length <= 1) {
            topics.forEach(item => {
              const option = document.createElement('option');
              option.value = item;
              option.textContent = item;
              topicSelect.appendChild(option);
            });
          }
          topicSelect.value = data.topic || topic || '';
          const radarURL = new URL('/studio/trends', window.location.origin);
          if (topicSelect.value) radarURL.searchParams.set('topic', topicSelect.value);
          const radarLink = hint.querySelector('a');
          if (radarLink) radarLink.href = radarURL.pathname + radarURL.search;

          const videos = Array.isArray(data.videos) ? data.videos : [];
          if (current && !videos.some(video => video.video_url === current)) {
            const saved = document.createElement('option');
            saved.value = current;
            saved.textContent = 'Nguồn đang lưu';
            select.appendChild(saved);
          }
          videos.forEach(video => {
            if (!video.video_url) return;
            const option = document.createElement('option');
            option.value = video.video_url;
            const status = video.dance_verified ? 'Đã xác nhận vũ đạo' : 'Chưa xác nhận vũ đạo';
            option.textContent = `${video.title || video.music || 'Video Radar'} · @${video.unique_id || video.author || 'TikTok'} · ${status}`;
            option.disabled = Boolean(node.params?.require_radar_dance) && !video.dance_verified;
            option.title = video.dance_verified ? 'Đã xác nhận trong Radar' : 'Mở Radar để xác nhận video nhảy';
            select.appendChild(option);
          });
          select.value = current;
          if (!videos.length) placeholder.textContent = 'Chủ đề chưa có video; mở Radar để cập nhật';
        } catch (_) {
          if (version === requestVersion) placeholder.textContent = 'Không tải được Radar; mở trang Radar để kiểm tra';
        }
      };
      topicSelect.addEventListener('change', () => loadTopic(topicSelect.value));
      loadTopic('');
    }

    customNodeGuidance(type) {
      const guidance = {
        custom_universal_ai: 'Dùng cho một node đa phương thức với cổng Text/Image/Video. Video chỉ được lấy frame tham chiếu; audio chưa được hỗ trợ.',
        text_to_speech: 'Nối kịch bản vào cổng Văn bản. Chọn persona ở cổng hoặc tham số; giọng thực tế tùy provider TTS. macOS Say hiện dùng giọng Linh. Văn bản dài tự chia theo câu rồi ghép thành một file WAV.',
        audio_mix: 'Nối lời đọc vào Track chính; nhạc nền là tùy chọn. Khi bật ducking, nhạc tự nhỏ xuống trong lúc có lời. Không nối nhạc thì node truyền nguyên track chính.',
        workflow_map: 'Nối một danh sách vào node. Chọn workflow con xử lý một phần tử; kết quả giữ nguyên thứ tự. Workflow con cần ít nhất một input và output.',
        custom_web_ai: 'Dùng khi cần chọn Google Flow hoặc Muse AI và truyền prompt cùng một file tham chiếu; output chọn Text, Image hoặc Video.',
        custom_ai_prompt: 'Dùng cho prompt tùy biến với system prompt, model hệ thống và cổng output riêng cho text/media.',
        custom_vision_analyzer: 'Phân tích ảnh hoặc một frame video; không phân tích toàn bộ chuyển động video.',
        custom_script_runner: 'Chỉ thay biến trong template văn bản; node không chạy mã tùy ý.',
        custom_export_publish: 'Xuất file để tải xuống; đăng lên TikTok cần workflow duyệt và bước publish riêng.'
      };
      return guidance[type] || '';
    }

    updateInspector() {
      const emptyEl = document.getElementById('inspector-empty');
      const formEl = document.getElementById('inspector-form');
      const tabsEl = document.getElementById('inspector-tabs');
      if (!emptyEl || !formEl) return;

      if (!this.selectedNodeId) {
        emptyEl.style.display = 'block';
        formEl.style.display = 'none';
        if (tabsEl) tabsEl.style.display = 'none';
        return;
      }

      const node = this.graph.nodes.find(n => n.id === this.selectedNodeId);
      if (!node) {
        emptyEl.style.display = 'block';
        formEl.style.display = 'none';
        if (tabsEl) tabsEl.style.display = 'none';
        return;
      }

      emptyEl.style.display = 'none';
      formEl.style.display = 'block';
      if (tabsEl) tabsEl.style.display = 'flex';
      this.setInspectorTab(this.activeInspectorTab || 'parameters');

      const def = this.definitions[node.type] || {};
      const unsupported = !this.definitions[node.type];
      const cat = node.category || def.category || 'process';

      const compatWarning = document.getElementById('inspector-compat-warning');
      if (compatWarning) {
        compatWarning.style.display = unsupported ? 'block' : 'none';
        compatWarning.textContent = unsupported
          ? 'Node này thuộc phiên bản cũ đã bị loại khỏi hệ thống. Các dây nối vẫn được giữ và hiển thị; node chưa thể chạy cho tới khi thay bằng node đang hỗ trợ.'
          : '';
      }
      const statusBadge = document.getElementById('inspector-node-status');
      if (statusBadge) statusBadge.innerHTML = this.renderNodeStatus(node.state || { status: 'idle' }, node.type);

      // 1. Icon & Badges
      const iconEl = document.getElementById('inspector-node-icon');
      if (iconEl) iconEl.innerText = this.getCategoryIcon(cat);

      const catEl = document.getElementById('inspector-node-category');
      if (catEl) catEl.innerText = cat.toUpperCase();

      const typeBadge = document.getElementById('inspector-node-type-badge');
      if (typeBadge) typeBadge.innerText = node.type;

      // 2. Editable Node Title (n8n / ComfyUI direct renaming)
      const titleInput = document.getElementById('inspector-node-title-input');
      if (titleInput) {
        titleInput.value = node.title || def.title || node.type;
        titleInput.title = titleInput.value;
        titleInput.oninput = () => {
          node.title = titleInput.value;
          titleInput.title = titleInput.value;
          const cardTitleEl = document.getElementById(`node-title-${node.id}`);
          if (cardTitleEl) {
            cardTitleEl.innerText = titleInput.value || def.title || node.type;
          }
        };
      }

      const descEl = document.getElementById('inspector-node-desc');
      if (descEl) descEl.innerText = def.description || '';
      const guidanceEl = document.getElementById('inspector-node-guidance');
      if (guidanceEl) {
        guidanceEl.textContent = this.customNodeGuidance(node.type);
        guidanceEl.style.display = guidanceEl.textContent ? 'block' : 'none';
      }

      // 3. LLM selector appears only where executor supports model choice.
      const modelGroup = document.getElementById('inspector-model-group');
      const modelSelect = document.getElementById('inspector-model-select');
      const supportsModel = Object.prototype.hasOwnProperty.call(def.default_params || {}, 'model');
      if (modelGroup && modelSelect) {
        modelGroup.style.display = supportsModel ? 'block' : 'none';
        modelSelect.value = node.params?.model || def.default_params?.model || 'active';
        modelSelect.onchange = () => {
          const m = modelSelect.value;
          if (!node.params) node.params = {};
          node.params.model = m;
          this.updateNodeModelBadge(node.id, m);
        };
      }

      // Show prompt fields only when this node definition actually supports them.
      const promptGroup = document.getElementById('inspector-prompt-group');
      const promptInput = document.getElementById('inspector-prompt-input');
      const promptLabel = document.getElementById('inspector-prompt-label');
      const promptHelp = document.getElementById('inspector-prompt-help');
      const charCounter = document.getElementById('inspector-prompt-char-count');
      const defaults = def.default_params || {};
      const params = node.params || {};
      const supportsPrompt = ['prompt', 'template'].some(key => Object.prototype.hasOwnProperty.call(defaults, key) || Object.prototype.hasOwnProperty.call(params, key));
      const supportsSystemPrompt = Object.prototype.hasOwnProperty.call(defaults, 'system_prompt') || Object.prototype.hasOwnProperty.call(params, 'system_prompt');
      const supportsNegativePrompt = Object.prototype.hasOwnProperty.call(defaults, 'negative_prompt') || Object.prototype.hasOwnProperty.call(params, 'negative_prompt');
      if (promptLabel && promptHelp) {
        if (node.type === 'prompt_template') {
          promptLabel.textContent = 'Mẫu prompt';
          promptHelp.textContent = 'Dùng {character}, {product}, {motion}; nội dung lấy từ các cổng text đầu vào.';
        } else if (node.type === 'custom_script_runner') {
          promptLabel.textContent = 'Mẫu văn bản';
          promptHelp.textContent = 'Dùng biến input được chèn bên dưới để lấy dữ liệu từ các cổng vào.';
        } else {
          promptLabel.textContent = 'Prompt riêng cho node';
          promptHelp.textContent = 'Để trống: dùng prompt qua dây nối. Nhập nội dung riêng để thay thế; chèn {{input}} để bổ sung prompt upstream.';
        }
      }
      const systemGroup = document.getElementById('inspector-system-prompt-group');
      const systemInput = document.getElementById('inspector-system-prompt-input');
      const negativeGroup = document.getElementById('inspector-negative-prompt-group');
      const negativeInput = document.getElementById('inspector-negative-prompt-input');
      if (promptGroup) promptGroup.style.display = supportsPrompt || supportsSystemPrompt || supportsNegativePrompt ? 'block' : 'none';
      if (promptInput) {
        promptInput.style.display = supportsPrompt ? 'block' : 'none';
        const promptValue = node.params?.prompt !== undefined
          ? node.params.prompt
          : (node.params?.template !== undefined ? node.params.template : (defaults.prompt || defaults.template || ''));
        promptInput.value = promptValue;
        promptInput.oninput = () => {
          const value = promptInput.value;
          if (!node.params) node.params = {};
          if (Object.prototype.hasOwnProperty.call(defaults, 'template') || Object.prototype.hasOwnProperty.call(node.params, 'template')) {
            node.params.template = value;
          } else {
            node.params.prompt = value;
          }
          if (charCounter) charCounter.innerText = `${value.length} ký tự`;
        };
        if (charCounter) charCounter.innerText = `${promptValue.length} ký tự`;
      }
      if (systemGroup && systemInput) {
        systemGroup.style.display = supportsSystemPrompt ? 'block' : 'none';
        systemInput.value = node.params?.system_prompt || defaults.system_prompt || '';
        systemInput.oninput = () => {
          if (!node.params) node.params = {};
          node.params.system_prompt = systemInput.value;
        };
      }
      if (negativeGroup && negativeInput) {
        negativeGroup.style.display = supportsNegativePrompt ? 'block' : 'none';
        negativeInput.value = node.params?.negative_prompt || defaults.negative_prompt || '';
        negativeInput.oninput = () => {
          if (!node.params) node.params = {};
          node.params.negative_prompt = negativeInput.value;
        };
      }
      const chips = document.getElementById('inspector-prompt-chips');
      if (chips) {
        chips.replaceChildren();
        const variables = this.promptVariablesForNode(node.type, def);
        chips.style.display = supportsPrompt && variables.length ? 'flex' : 'none';
        variables.forEach(variable => {
          const chip = document.createElement('button');
          chip.type = 'button';
          chip.className = 'wf-chip-btn';
          chip.textContent = `+ ${variable}`;
          chip.addEventListener('click', () => {
            const start = promptInput.selectionStart ?? promptInput.value.length;
            const end = promptInput.selectionEnd ?? promptInput.value.length;
            promptInput.setRangeText(variable, start, end, 'end');
            promptInput.dispatchEvent(new Event('input', { bubbles: true }));
            promptInput.focus();
          });
          chips.appendChild(chip);
        });
      }

      // 5. Dynamic Other Parameters (Exclude prompt, template, model, provider to prevent duplication)
      const fieldsContainer = document.getElementById('inspector-fields');
      if (fieldsContainer) {
        fieldsContainer.innerHTML = '';
        if (!node.params) node.params = {};
        const params = Object.assign({}, def.default_params, node.params);
		const isWorkflowBoundary = node.type === 'workflow_input' || node.type === 'workflow_output';
		const isWorkflowCall = this.isWorkflowCallType(node.type);
		const isWorkflowMap = this.isWorkflowMapType(node.type);
		if (isWorkflowBoundary) this.renderWorkflowBoundaryInspector(node, fieldsContainer);
		else if (isWorkflowCall) this.renderWorkflowCallInspector(node, fieldsContainer);
		else if (isWorkflowMap) this.renderWorkflowMapInspector(node, fieldsContainer);
        else if (node.type === 'virtual_tryon') {
          const guidance = document.createElement('p');
          guidance.className = 'wf-inspector-section-help';
          guidance.textContent = 'Muốn đổi phong cách mỗi lần chạy? Thêm node Đầu vào workflow, khai báo lựa chọn, rồi nối vào cổng “Phong cách trang phục”.';
          fieldsContainer.appendChild(guidance);
        }

        const skipKeys = ['prompt', 'template', 'system_prompt', 'negative_prompt', 'model', 'node_name', 'preview_url', 'workflow_id', 'workflow_name', 'item_input_key', 'index_input_key', 'result_output_key'];

        if (!isWorkflowBoundary && !isWorkflowCall) Object.keys(params).forEach(key => {
          if (skipKeys.includes(key) || (node.type === 'virtual_tryon' && key === 'fit_style')) return;

          const val = params[key];
          const group = document.createElement('div');
          group.className = 'form-group wf-inspector-param-group';

          const label = document.createElement('label');
          label.style.display = 'block';
          label.style.fontWeight = '600';
          label.style.fontSize = '12px';
          label.style.marginBottom = '4px';
          label.innerText = this.formatParamLabel(key);
          group.appendChild(label);

          const choiceSelect = this.createChoiceSelect(key, val, node, `inspector-field-${key}`);
          const sliderControl = this.createSliderOrNumberInput(key, val, node, `inspector-field-${key}`, (newVal) => {
            node.params[key] = newVal;
          });

          if (key === 'product_id' && node.type === 'product_input') {
            group.appendChild(this.createProductSelect(node, val, `inspector-field-${key}`));
            const note = document.createElement('small');
            note.className = 'text-muted';
            note.textContent = 'Chỉ hiển thị sản phẩm đã duyệt và đủ dữ liệu affiliate.';
            group.appendChild(note);
          } else if (choiceSelect) {
            choiceSelect.addEventListener('change', () => { node.params[key] = choiceSelect.value; });
            group.appendChild(choiceSelect);
          } else if (sliderControl) {
            group.appendChild(sliderControl);
          } else if (typeof val === 'boolean') {
            const chkWrap = document.createElement('label');
            chkWrap.style.cssText = 'display:flex;align-items:center;gap:8px;cursor:pointer;margin-top:4px;';

            const chk = document.createElement('input');
            chk.type = 'checkbox';
            chk.checked = val;
            chk.addEventListener('change', () => {
              node.params[key] = chk.checked;
            });
            chkWrap.appendChild(chk);

            const span = document.createElement('span');
            span.innerText = 'Bật / Kích hoạt chức năng này';
            span.style.fontSize = '12.5px';
            chkWrap.appendChild(span);

            group.appendChild(chkWrap);
          } else if (typeof val === 'string' && key === 'task') {
            const input = document.createElement('textarea');
            input.className = 'form-control';
            input.rows = 3;
            input.value = val;
            input.placeholder = 'Để trống để nhận chỉ thị từ cổng task';
            input.setAttribute('aria-label', this.formatParamLabel(key));
            input.addEventListener('input', () => { node.params[key] = input.value; });
            group.appendChild(input);
          } else if (typeof val === 'string' && /prompt|template/i.test(key)) {
            const input = document.createElement('textarea');
            input.className = 'wf-prompt-textarea';
            input.rows = 5;
            input.value = val;
            input.placeholder = 'Nhập prompt riêng; dùng {{input}} để chèn nội dung từ node trước.';
            input.setAttribute('aria-label', this.formatParamLabel(key));
            input.addEventListener('input', () => { node.params[key] = input.value; });
            group.appendChild(input);
          } else if (typeof val === 'number') {
            const numInput = document.createElement('input');
            numInput.type = 'number';
            numInput.className = 'form-control';
            numInput.value = val;
            numInput.addEventListener('input', () => {
              node.params[key] = parseFloat(numInput.value) || 0;
            });
            group.appendChild(numInput);
          } else if (this.isMediaParameter(key)) {
            const row = document.createElement('div');
            row.style.cssText = 'display:flex;gap:6px;align-items:center;';

            const input = document.createElement('input');
            input.type = 'text';
            input.className = 'form-control';
            input.id = `inspector-field-${key}`;
            input.style.flex = '1';
            input.value = val;
            input.placeholder = key.includes('url') ? 'https://www.tiktok.com/... hoặc link ảnh/video' : 'Đường dẫn tệp...';
            input.addEventListener('input', () => {
              node.params[key] = input.value;
              if (node.type === 'video_source' && key === 'video_url') this.clearVideoSourceOtherParam(node, key);
            });
            if (node.type === 'video_source' && key === 'video_url') this.appendRadarPicker(group, node, input);
            row.appendChild(input);

            // Nút dán link / fetch URL
            const fetchBtn = document.createElement('button');
            fetchBtn.type = 'button';
            fetchBtn.className = 'btn btn-secondary';
            fetchBtn.style.cssText = 'padding:4px 8px;font-size:11px;white-space:nowrap;';
            fetchBtn.title = 'Tải media từ link TikTok, Reels, IG, Web';
            fetchBtn.innerHTML = '🔗 Link';
            if (node.type === 'video_source' && key === 'video_url') {
              fetchBtn.title = 'Lưu URL; workflow xác thực Radar hoặc quyền URL trực tiếp khi chạy';
              fetchBtn.innerHTML = '↗ URL';
            }
            fetchBtn.addEventListener('click', async () => {
              const u = await window.showAppPrompt('Dán link video TikTok, Instagram Reels, IG, YouTube Shorts hoặc link ảnh/video web:', input.value || '', { label: 'Đường dẫn media', placeholder: 'https://…', confirmText: 'Tải media' });
              if (u && u.trim()) {
                input.value = u.trim();
                await this.fetchMediaFromUrl(node, u.trim(), this.preferredMediaType(key), key);
              }
            });
            row.appendChild(fetchBtn);

            // Nút tải tệp từ máy
            const uploadBtn = document.createElement('button');
            uploadBtn.type = 'button';
            uploadBtn.className = 'btn btn-secondary';
            uploadBtn.style.cssText = 'padding:4px 8px;font-size:11px;white-space:nowrap;';
            uploadBtn.title = 'Tải tệp từ máy tính';
            uploadBtn.innerHTML = '📁 Tải';

            const hiddenFile = document.createElement('input');
            hiddenFile.type = 'file';
            hiddenFile.accept = key.includes('audio') ? 'audio/*' : (key.includes('video') ? 'video/*' : (key === 'media_path' ? 'image/*,video/*,audio/*' : 'image/*'));
            hiddenFile.style.display = 'none';
            hiddenFile.addEventListener('change', async (fev) => {
              const file = fev.target.files && fev.target.files[0];
              if (!file) return;
              try {
                uploadBtn.disabled = true;
                uploadBtn.innerText = '⏳';
                const fd = new FormData();
                fd.append('file', file);
                const r = await fetch('/api/flow/upload', { method: 'POST', body: fd });
                const d = await r.json();
                if (d.ok) {
                  input.value = d.file_path;
                  node.params[key] = d.file_path;
                  if (node.type === 'video_source' && key === 'video_path') this.clearVideoSourceOtherParam(node, key);
                  node.params.preview_url = d.url;
                  this.updateNodePreview(node.id, d.url);
                }
              } catch (uErr) {
                console.error(uErr);
              } finally {
                uploadBtn.disabled = false;
                uploadBtn.innerHTML = '📁 Tải';
              }
            });

            uploadBtn.addEventListener('click', () => hiddenFile.click());
            row.appendChild(uploadBtn);
            row.appendChild(hiddenFile);

            group.appendChild(row);
          } else {
            const input = document.createElement('input');
            input.type = 'text';
            input.className = 'form-control';
            input.id = `inspector-field-${key}`;
            input.value = val;
            input.addEventListener('input', () => {
              node.params[key] = input.value;
            });
            group.appendChild(input);
          }

          fieldsContainer.appendChild(group);
        });
      }

      this.renderInspectorPorts(node);
      const outputLog = document.getElementById('inspector-output-json');
      if (outputLog) {
        outputLog.textContent = node.state && (node.state.output || node.state.outputs || node.state.error || node.state.status)
          ? JSON.stringify(node.state, null, 2)
          : 'Node chưa chạy.';
      }

      // 6. Preview most recent media or text output.
      const previewBox = document.getElementById('inspector-preview-box');
      const previewStatus = document.getElementById('inspector-preview-status');
      if (previewBox) {
        const stateOutput = node.state?.outputs || node.state?.output || {};
        const mediaPath = node.state?.output_preview || this.findOutputMediaPath(stateOutput);
        const mediaName = this.getFilename(mediaPath);
        const textValue = typeof stateOutput === 'string'
          ? stateOutput
          : (stateOutput?.text_out || stateOutput?.text || stateOutput?.prompt_out || '');
        if (mediaPath) {
          previewBox.classList.add('has-content');
          if (previewStatus) previewStatus.innerText = 'Đã có kết quả';
          const isVideo = /\.(mp4|webm|mov|mkv|avi)$/i.test(mediaName);
          const isAudio = /\.(mp3|aac|wav|ogg|m4a|flac)$/i.test(mediaName);
          const media = document.createElement(isAudio ? 'audio' : (isVideo ? 'video' : 'img'));
          const pathValue = String(mediaPath);
          const isServedURL = /^https?:\/\//i.test(pathValue) || /^\/(media|static|uploads)\//i.test(pathValue);
          const mediaURL = isServedURL
            ? safeMediaURL(pathValue)
            : safeMediaURL(`/media/${encodeURIComponent(mediaName)}`);
          media.src = mediaURL;
          media.style.maxWidth = '100%';
          media.style.borderRadius = '6px';
          if (isAudio) {
            media.controls = true;
            media.style.width = '100%';
          } else if (isVideo) {
            media.controls = true;
            media.muted = true;
            media.playsInline = true;
          } else {
            media.alt = '';
          }
          media.onerror = () => { media.style.display = 'none'; };
          previewBox.replaceChildren(media);
        } else if (textValue) {
          previewBox.classList.add('has-content');
          if (previewStatus) previewStatus.innerText = 'Đã tạo văn bản';
          const outputText = document.createElement('div');
          outputText.style.cssText = 'color:var(--text);font-size:12px;white-space:pre-wrap;line-height:1.45;padding:4px;';
          outputText.textContent = String(textValue);
          previewBox.replaceChildren(outputText);
        } else {
          previewBox.classList.remove('has-content');
          if (previewStatus) previewStatus.innerText = 'Chưa có kết quả';
          previewBox.innerHTML = '<span class="text-muted">Chưa có dữ liệu xuất</span>';
        }
      }
    }

    promptVariablesForNode(type, def) {
      const variables = {
        prompt_template: ['{character}', '{motion}', '{product}'],
        custom_script_runner: ['{{input_1}}', '{{input_2}}', '{{text_in}}']
      };
      if (variables[type]) return variables[type];
      const inputs = Array.isArray(def?.inputs) ? def.inputs : [];
      return Array.from(new Set([
        '{{input}}',
        ...inputs.filter(port => port.type === 'text').map(port => `{{${port.id}}}`)
      ]));
    }

    renderInspectorPorts(node) {
      const edges = this.graph.edges || [];
      const renderList = (containerId, ports, isOutput) => {
        const container = document.getElementById(containerId);
        if (!container) return;
        if (!ports.length) {
          container.innerHTML = '<span class="wf-inspector-empty-port">Không có cổng.</span>';
          return;
        }
        container.innerHTML = ports.map(port => {
          const connected = edges.filter(edge => {
            const ownerId = isOutput
              ? (edge.from_node || edge.fromNode || edge.FromNode)
              : (edge.to_node || edge.toNode || edge.ToNode);
            const connectedPort = isOutput
              ? (edge.from_port || edge.fromPort || edge.FromPort)
              : (edge.to_port || edge.toPort || edge.ToPort);
            return ownerId === node.id && connectedPort === port.id;
          });
          const peers = connected.map(edge => {
            const peerId = isOutput
              ? (edge.to_node || edge.toNode || edge.ToNode)
              : (edge.from_node || edge.fromNode || edge.FromNode);
            const peer = this.graph.nodes.find(candidate => candidate.id === peerId);
            return escapeHTML(peer?.title || peer?.type || peerId || 'Node');
          });
          const connectionText = peers.length
            ? peers.join(', ')
            : '<span class="wf-inspector-unconnected">Chưa nối</span>';
          const legacyClass = port.legacy ? ' is-legacy' : '';
          return `<div class="wf-inspector-port-row${legacyClass}"><div><strong>${escapeHTML(port.name || port.id)}</strong><code>${escapeHTML(port.type || 'any')}</code></div><span>${connectionText}</span></div>`;
        }).join('');
      };

      renderList('inspector-inputs-list', this.getNodePorts(node, false), false);
      renderList('inspector-outputs-list', this.getNodePorts(node, true), true);
    }

    formatModelShortName(model) {
      if (!model) return '';
      const map = {
        'gemini-3.8-flash': 'Gemini 3.8',
        'gemini-3.1-pro': 'Gemini 3.1 Pro',
        'gemini_2_5_flash': 'Gemini 3.8',
        'gemini_2_5_pro': 'Gemini 3.1 Pro',
        'gpt-6-luna': 'GPT-6 Luna',
        'claude-5-5-sonet': 'Claude 5.5',
        'claude_5_5_sonet': 'Claude 5.5',
        'grok-3': 'Grok 3',
        'veo-3.1': 'Veo 3.1',
        'muse-gen-2': 'Muse Gen-2',
        'active': 'Hệ thống',
        'auto': 'Auto LLM',
      };
      return map[model.toLowerCase()] || model;
    }

    updateNodeModelBadge(nodeId, model) {
      const badge = document.getElementById(`node-model-${nodeId}`);
      if (badge) {
        if (model) {
          badge.innerText = this.formatModelShortName(model);
          badge.style.display = '';
        } else {
          badge.style.display = 'none';
        }
      }
    }

    openExportApiModal() {
      this.syncGraphMetadataFromHeader();
      if (!this.graphPersisted || this.isGraphDirty()) {
        window.showAppNotice('Lưu workflow trước khi export API để endpoint chạy đúng phiên bản đã lưu.');
        return;
      }
      const modal = document.getElementById('export-api-modal');
      if (!modal) return;

      const urlEl = document.getElementById('api-endpoint-url');
      const origin = window.location.origin || 'http://localhost:8080';
      const apiUrl = `${origin}/api/flow/api-run`;
      if (urlEl) urlEl.innerText = apiUrl;

      // Auto-detect ALL input parameters across the workflow
      const detectedFlatParams = {};
      const detectedNodeParams = {};
      const detectedChips = [];

      let charImageCount = 0;
      let productImageCount = 0;
      let otherImageCount = 0;
      let videoCount = 0;
      let promptCount = 0;

      const workflowInputs = (this.graph.nodes || []).filter(node => node.type === 'workflow_input');
      if (workflowInputs.length) {
        try {
          this.getWorkflowContract(this.graph);
        } catch (error) {
          window.showAppNotice('Không thể export workflow: ' + error.message);
          return;
        }
        workflowInputs.forEach(node => {
          const params = node.params || {};
          const key = String(params.key || '').trim();
          const type = String(params.value_type || 'any').toUpperCase();
          const samples = {
            IMAGE: 'https://example.com/image.jpg',
            IMAGE_LIST: ['https://example.com/image-1.jpg'],
            VIDEO: 'https://example.com/video.mp4',
            AUDIO: 'https://example.com/audio.mp3',
            TEXT: params.value || '',
            NUMBER: 1,
            PRODUCT: {},
            MOTION: {},
            CONFIG: {},
            ANY: params.value || ''
          };
          detectedFlatParams[key] = Object.prototype.hasOwnProperty.call(samples, type) ? samples[type] : '';
          detectedChips.push({ name: key, type, nodeRef: node.title || key, desc: params.required === false ? 'Không bắt buộc' : 'Bắt buộc' });
        });
        const guide = document.getElementById('api-guide-text');
        if (guide) guide.innerText = 'Truyền đúng key đã khai báo ở node Đầu vào. Khi chạy xong, đọc outputs trong status_url để lấy các node Đầu ra.';
      } else (this.graph.nodes || []).forEach(n => {
        const type = (n.type || '').toLowerCase();
        const cat = (n.category || '').toLowerCase();
        const params = n.params || {};
        const title = n.title || n.id;

        // 1. Character / Avatar Image Input
        if (type.includes('character') || type.includes('avatar') || type.includes('identity')) {
          charImageCount++;
          const paramKey = charImageCount === 1 ? 'character_image_url' : `character_image_url_${charImageCount}`;
          const val = params.image_url || params.source_image || params.character_image || 'https://example.com/assets/model_face.jpg';
          detectedFlatParams[paramKey] = val;
          detectedChips.push({
            name: paramKey,
            type: 'IMAGE',
            nodeRef: title,
            desc: `Ảnh người mẫu / Face ID`
          });
          detectedNodeParams[n.id] = { image_url: val };
        }
        // 2. Product / Garment / Fabric Image Input
        else if (type.includes('product') || type.includes('garment') || type.includes('fabric') || type.includes('cloth')) {
          productImageCount++;
          const paramKey = productImageCount === 1 ? 'product_image_url' : `product_image_url_${productImageCount}`;
          const val = params.image_url || params.source_image || params.product_image || 'https://example.com/assets/product_garment.png';
          detectedFlatParams[paramKey] = val;
          if (params.product_title || type.includes('product')) {
            detectedFlatParams['product_name'] = params.product_title || 'Sản phẩm mẫu';
          }
          detectedChips.push({
            name: paramKey,
            type: 'IMAGE',
            nodeRef: title,
            desc: `Ảnh sản phẩm / vải`
          });
          detectedNodeParams[n.id] = { image_url: val };
        }
        // 3. Video Input (Motion, Pose, Source Video)
        else if (type.includes('video') || type.includes('pose') || type.includes('skeleton') || params.video_url || params.source_video) {
          videoCount++;
          const paramKey = videoCount === 1 ? 'video_url' : `video_url_${videoCount}`;
          const val = params.video_url || params.source_video || 'https://www.tiktok.com/@trend/video/123456';
          detectedFlatParams[paramKey] = val;
          detectedChips.push({
            name: paramKey,
            type: 'VIDEO',
            nodeRef: title,
            desc: `Video chuyển động / Pose`
          });
          detectedNodeParams[n.id] = { video_url: val };
        }
        // 4. Prompt Input (Script, Scenario, AI Prompt)
        else if (type.includes('prompt') || type.includes('script') || (params.prompt && cat === 'input')) {
          promptCount++;
          const paramKey = promptCount === 1 ? 'prompt' : `prompt_${promptCount}`;
          const val = params.prompt || 'Video ngắn giới thiệu thời trang phong cách năng động...';
          detectedFlatParams[paramKey] = val;
          detectedChips.push({
            name: paramKey,
            type: 'TEXT',
            nodeRef: title,
            desc: `Prompt kịch bản AI`
          });
          detectedNodeParams[n.id] = { prompt: val };
        }
        // 5. Generic Image Input
        else if (params.image_url || params.source_image) {
          otherImageCount++;
          const paramKey = `image_url_${otherImageCount}`;
          const val = params.image_url || params.source_image || 'https://example.com/assets/sample.jpg';
          detectedFlatParams[paramKey] = val;
          detectedChips.push({
            name: paramKey,
            type: 'IMAGE',
            nodeRef: title,
            desc: `Ảnh đầu vào`
          });
          detectedNodeParams[n.id] = { image_url: val };
        }
      });
      if (!workflowInputs.length) {
        const guide = document.getElementById('api-guide-text');
        if (guide) guide.innerText = 'Khi có nhiều ảnh / video: truyền theo tên tham số chuẩn hoặc truyền theo node_id tại tab JSON (Theo Node ID).';
      }

      // Default fallback if no specific input nodes detected
      if (Object.keys(detectedFlatParams).length === 0) {
        detectedFlatParams['prompt'] = 'Video ngắn giới thiệu thời trang phong cách năng động';
        detectedChips.push({ name: 'prompt', type: 'TEXT', nodeRef: 'Workflow', desc: 'Nội dung Prompt' });
      }

      // Render parameter chips
      const paramsListEl = document.getElementById('api-detected-params-list');
      if (paramsListEl) {
        paramsListEl.innerHTML = detectedChips.map(c => `
          <div class="wf-api-param-chip" title="${escapeHTML(c.nodeRef)}">
            <span class="wf-api-param-key">${escapeHTML(c.name)}</span>
            <span class="wf-api-param-type">${escapeHTML(c.type)}</span>
            <span class="wf-api-param-noderef">[${escapeHTML(c.nodeRef.slice(0, 24))}]</span>
          </div>
        `).join('');
      }

      const samplePayloadFlat = {
        graph_id: this.graph.id,
        params: detectedFlatParams
      };

      const samplePayloadNode = {
        graph_id: this.graph.id,
        params: workflowInputs.length ? detectedFlatParams : (Object.keys(detectedNodeParams).length > 0 ? detectedNodeParams : detectedFlatParams)
      };

      if (flowGraphMayUseProviderCredits(this.graph)) {
        samplePayloadFlat.muse_credits_acknowledged = false;
        samplePayloadNode.muse_credits_acknowledged = false;
        samplePayloadFlat.google_flow_credits_acknowledged = false;
        samplePayloadNode.google_flow_credits_acknowledged = false;
        const guide = document.getElementById('api-guide-text');
        if (guide) guide.innerText = 'Workflow có thể dùng Muse AI hoặc Google Flow credits. Chỉ đặt trường xác nhận provider tương ứng thành true khi bên gọi đã cho phép.';
      }

      const jsonStrFlat = JSON.stringify(samplePayloadFlat, null, 2);
      const jsonStrNode = JSON.stringify(samplePayloadNode, null, 2);

      const reqBodyEl = document.getElementById('api-request-body');
      if (reqBodyEl) reqBodyEl.value = jsonStrFlat;

      const reqBodyNodeEl = document.getElementById('api-request-body-node');
      if (reqBodyNodeEl) reqBodyNodeEl.value = jsonStrNode;

      const curlCodeEl = document.getElementById('api-curl-code');
      if (curlCodeEl) {
        curlCodeEl.innerText = `curl -X POST ${apiUrl} \\\n  -H "Content-Type: application/json" \\\n  -d '${jsonStrFlat}'`;
      }

      const respWrap = document.getElementById('api-test-response-wrap');
      if (respWrap) respWrap.style.display = 'none';
      const statusEl = document.getElementById('api-test-status');
      if (statusEl) statusEl.innerText = '';

      modal.style.display = 'flex';
    }

    closeExportApiModal() {
      const modal = document.getElementById('export-api-modal');
      if (modal) modal.style.display = 'none';
    }

    async runApiTest() {
      const statusEl = document.getElementById('api-test-status');
      const respWrap = document.getElementById('api-test-response-wrap');
      const respCode = document.getElementById('api-test-response-code');
      const respBadge = document.getElementById('api-response-status-badge');
      const reqBodyEl = document.getElementById('api-request-body');
      const reqBodyNodeEl = document.getElementById('api-request-body-node');
      const tabBtnJsonNode = document.getElementById('tab-btn-json-node');
      const btnTest = document.getElementById('btn-run-api-test');

      const isNodeTab = tabBtnJsonNode && tabBtnJsonNode.classList.contains('active');
      const targetTextarea = isNodeTab && reqBodyNodeEl ? reqBodyNodeEl : reqBodyEl;

      if (!targetTextarea) return;
      let payload;
      try {
        payload = JSON.parse(targetTextarea.value);
      } catch (err) {
        window.showAppNotice('Cú pháp JSON không hợp lệ: ' + err.message);
        return;
      }

      if (flowGraphMayUseProviderCredits(this.graph)) {
        if (!window.confirm('API test này có thể gửi nội dung tới Muse AI hoặc Google Flow và dùng credits của tài khoản. Chỉ tiếp tục nếu bạn cho phép dùng credits. Tiếp tục?')) return;
        payload.muse_credits_acknowledged = true;
        payload.google_flow_credits_acknowledged = true;
      }

      if (statusEl) {
        statusEl.style.color = '#6366F1';
        statusEl.innerText = '⏳ Đang gửi request tới API...';
      }
      if (btnTest) btnTest.disabled = true;

      try {
        const resp = await fetch('/api/flow/api-run', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(payload)
        });
        const data = await resp.json();

        if (respWrap && respCode) {
          respWrap.style.display = 'block';
          respCode.innerText = JSON.stringify(data, null, 2);
        }

        if (respBadge) {
          if (resp.ok && data.ok) {
            respBadge.className = 'wf-badge wf-badge-success';
            respBadge.innerText = '200 OK';
          } else {
            respBadge.className = 'wf-badge wf-badge-danger';
            respBadge.innerText = `${resp.status || 400} Error`;
          }
        }

        if (data.ok && data.run_id) {
          if (statusEl) {
            statusEl.style.color = '#10B981';
            statusEl.innerText = `✓ API đã nhận lệnh! Run ID: ${data.run_id}`;
          }
          this.activeRun = { id: data.run_id, graph_id: payload.graph_id, status: 'running' };
          if (this.statusPill) {
            this.statusPill.className = 'wf-status-pill status-running';
            this.statusPill.innerText = `⚡ API Run: ${data.run_id.slice(-8)}`;
          }
          this.currentRunId = data.run_id;
          const runPanel = document.getElementById('flow-run-panel');
          if (runPanel) runPanel.hidden = false;
          this.clearLogs();
          this.renderWorkflowOutputs({});
          this.startPollingRun(data.run_id);
        } else {
          if (statusEl) {
            statusEl.style.color = '#EF4444';
            statusEl.innerText = `❌ Lỗi: ${data.error || 'Thất bại'}`;
          }
        }
      } catch (e) {
        if (statusEl) {
          statusEl.style.color = '#EF4444';
          statusEl.innerText = `❌ Lỗi kết nối: ${e.message}`;
        }
        if (respBadge) {
          respBadge.className = 'wf-badge wf-badge-danger';
          respBadge.innerText = 'Connection Failed';
        }
      } finally {
        if (btnTest) btnTest.disabled = false;
      }
    }

    formatParamLabel(key) {
      const labels = {
        voice: 'Phong cách giọng đọc',
        chunk_size: 'Ký tự tối đa mỗi đoạn đọc',
        max_items: 'Số phần tử tối đa mỗi lần chạy',
        voice_volume: 'Âm lượng track chính',
        music_volume: 'Âm lượng nhạc nền',
        duck_music: 'Giảm nhạc khi có lời đọc',
        loop_music: 'Lặp nhạc đến hết track chính',
        normalize: 'Chuẩn hóa âm lượng đầu ra',
        template: 'Mẫu câu lệnh (Template)',
        prompt: 'Câu lệnh Prompt & Chỉ dẫn AI',
        system_prompt: 'Chỉ đạo vai trò AI (System Prompt)',
        negative_prompt: 'Prompt phủ định (Negative Prompt)',
        model_name: 'Tên nhân vật mẫu',
        product_title: 'Tên sản phẩm',
        aspect: 'Tỉ lệ khung hình (Aspect)',
        fit_mode: 'Cách khớp khung hình',
        placement: 'Vị trí ghép sản phẩm',
        scale: 'Tỉ lệ phóng to (Scale)',
        resolution: 'Độ phân giải',
        expression: 'Biểu cảm & Điệu nhảy',
        provider_name: 'Tên Web AI',
        target_url: 'Đường dẫn đăng nhập AI',
        auto_publish_draft: 'Tự động xuất TikTok nháp',
        url: 'Đường dẫn liên kết (TikTok/Reels/IG/Web)',
        video_url: 'Đường dẫn video mạng xã hội',
        video_path: 'Tệp video mẫu',
        model_path: 'Tệp ảnh nhân vật',
        character_image: 'Ảnh nhân vật mẫu',
        product_image: 'Ảnh sản phẩm mẫu',
        source_image: 'Ảnh / Video nguồn bóc tách',
        source_video: 'Video nguồn bóc tách',
        video_timestamp: 'Thời điểm trích khung hình video (giây)',
        confidence_threshold: 'Độ nhạy bóc tách nền (Threshold)',
        feather_radius: 'Độ làm mềm viền cắt (Feather)',
        invert_mask: 'Đảo ngược mặt nạ chọn (Mask)',
        contrast_boost: 'Tăng tương phản (Contrast Boost)',
        saturation: 'Độ bão hòa màu (Saturation)',
        target_resolution: 'Độ phân giải Master (Resolution)',
        fps: 'Tốc độ khung hình (FPS)',
        shot_style: 'Phong cách góc máy điện ảnh',
        location_type: 'Bối cảnh không gian (Location)',
        custom_location: 'Mô tả không gian tùy chỉnh',
        depth_of_field: 'Độ sâu trường ảnh xóa phông (Depth of Field)',
        time_of_day: 'Thời gian & Ánh sáng (Time & Lighting)',
        color_temperature: 'Nhiệt độ màu (Color Temp)',
        light_intensity: 'Cường độ ánh sáng (Intensity)',
        dance_style: 'Phong cách vũ đạo (Dance Style)',
        dance_type: 'Loại hình vũ đạo (Dance Type)',
        motion_fidelity: 'Độ khớp động tác gốc (Fidelity)',
        fit_style: 'Phong cách trang phục (Fit Style)',
        outfit_name: 'Tên trang phục biểu diễn / thời trang',
        output_type: 'Kiểu kết quả đầu ra (Output Type)',
        task: 'Chỉ thị phân tích riêng (để trống dùng cổng task)',
        analysis_task: 'Nhiệm vụ phân tích thị giác',
        operation: 'Tác vụ xử lý media (Operation)',
        aspect_ratio: 'Tỷ lệ khung hình (Aspect Ratio)',
        filter_preset: 'Bộ lọc màu điện ảnh (Color Filter)',
        webhook_url: 'Đường dẫn Webhook / API URL',
        method: 'Phương thức HTTP (Method)',
        auth_token: 'Mã xác thực Bearer Token',
        mode: 'Chế độ xử lý logic (Mode)',
        media_title: 'Tiêu đề tài nguyên media',
        filename_prefix: 'Tiền tố tên file xuất xưởng',
        fabric_type: 'Chất liệu vải 3D (Fabric Material)',
        wrinkle_depth: 'Độ nhăn vải & nếp gấp (Wrinkle Depth)',
        stiffness: 'Độ cứng phom vải (Fabric Stiffness)',
        drape_flow: 'Độ rủ & Sóng vải (Drape & Flow)',
        cloth_tension: 'Độ căng bề mặt vải (Cloth Tension)',
        gravity_factor: 'Trọng lực mô phỏng (Gravity Factor)',
        gravity_simulation: 'Mô phỏng trọng lực vật lý (Physics Gravity)',
        wind_force: 'Lực gió tương tác (Wind Force)',
        simulation_quality: 'Chất lượng mô phỏng 3D (Simulation Quality)',
        lut_profile: 'Hồ sơ màu sắc 3D LUT (Color Profile)',
        blend_ratio: 'Tỷ lệ hòa trộn khuôn mặt (Face Blend)',
        preserve_expression: 'Giữ nguyên biểu cảm gốc (Preserve Expression)',
        face_similarity_threshold: 'Ngưỡng nhận diện khuôn mặt (Similarity)',
        gender: 'Giới tính nhân vật (Gender)',
        pose_style: 'Kiểu dáng chuyển động (Pose Style)',
        skeleton_precision: 'Độ chính xác khung xương (Skeleton Precision)',
        aperture: 'Khẩu độ ống kính (Aperture)',
        focal_length: 'Tiêu cự máy quay (Focal Length)',
        camera_motion: 'Chuyển động máy quay (Camera Motion)',
        key_light_lux: 'Cường độ đèn chính (Key Light Lux)',
        key_position: 'Góc đặt đèn chính (Key Light Angle)',
        fill_ratio: 'Tỉ lệ đèn phụ Fill Light (Fill Ratio)',
        rim_color_kelvin: 'Nhiệt độ màu đèn ven (Rim Kelvin)',
        mood: 'Bầu không khí ánh sáng (Lighting Mood)',
        motion_smoothness: 'Độ mượt chuyển động (Motion Smoothness)',
        rendering_engine: 'Công cụ tạo video (AI Render Engine)',
        bitrate_mode: 'Chế độ nén Bitrate (Bitrate Mode)',
        keyframe_count: 'Số khung hình trích xuất (Keyframes)',
        require_audio: 'Yêu cầu có âm thanh (Require Audio)',
        refine_edges: 'Làm mịn viền bóc tách (Refine Edges)',
        auto_crop: 'Tự động căn chỉnh & cắt khung (Auto Crop)',
        affiliate_url: 'Đường dẫn liên kết Affiliate (URL)'
      };
      if (labels[key]) return labels[key];
      return String(key).replace(/_/g, ' ').replace(/\b\w/g, l => l.toUpperCase());
    }

    openAddNodeModal() {
      const modal = document.getElementById('add-node-modal');
      if (modal) {
        modal.style.display = 'flex';
        const categories = document.querySelectorAll('#node-categories-list .wf-tag-filter');
        categories.forEach(chip => {
          const active = chip.dataset.cat === 'all';
          chip.classList.toggle('active', active);
          chip.setAttribute('aria-pressed', String(active));
        });
        const categoryScroller = document.getElementById('node-categories-list');
        if (categoryScroller) categoryScroller.scrollLeft = 0;
        const searchInput = document.getElementById('node-search-input');
        if (searchInput) {
          searchInput.value = '';
          searchInput.focus();
        }
        this.renderModalGrid('', 'all');
        requestAnimationFrame(() => this.syncNodeCategoryScrollButtons());
      }
    }

    syncNodeCategoryScrollButtons() {
      const scroller = document.getElementById('node-categories-list');
      const previous = document.getElementById('node-categories-prev');
      const next = document.getElementById('node-categories-next');
      if (!scroller || !previous || !next) return;
      const maxScroll = Math.max(0, scroller.scrollWidth - scroller.clientWidth);
      previous.disabled = scroller.scrollLeft <= 1;
      next.disabled = scroller.scrollLeft >= maxScroll - 1;
    }

    revealNodeCategory(chip) {
      const scroller = document.getElementById('node-categories-list');
      if (!scroller || !chip) return;
      const scrollerRect = scroller.getBoundingClientRect();
      const chipRect = chip.getBoundingClientRect();
      if (chipRect.left < scrollerRect.left) {
        scroller.scrollBy({ left: chipRect.left - scrollerRect.left - 8, behavior: 'smooth' });
      } else if (chipRect.right > scrollerRect.right) {
        scroller.scrollBy({ left: chipRect.right - scrollerRect.right + 8, behavior: 'smooth' });
      }
    }

    showAllNodes() {
      this.pendingConnect = null;
      const modal = document.getElementById('add-node-modal');
      const searchInput = modal?.querySelector('#node-search-input');
      if (searchInput) searchInput.value = '';
      const categoryList = modal?.querySelector('#node-categories-list');
      const allChip = categoryList?.querySelector('.wf-tag-filter[data-cat="all"]');
      categoryList?.querySelectorAll('.wf-tag-filter').forEach(chip => {
        const active = chip === allChip;
        chip.classList.toggle('active', active);
        chip.setAttribute('aria-pressed', String(active));
      });
      if (categoryList) categoryList.scrollLeft = 0;
      this.renderModalGrid('', 'all');
      this.syncNodeCategoryScrollButtons();
    }

    closeAddNodeModal() {
      const modal = document.getElementById('add-node-modal');
      if (modal) {
        modal.style.display = 'none';
      }
      this.pendingConnect = null;
    }

    bindModal() {
      const modal = document.getElementById('add-node-modal');
      const closeBtn = document.getElementById('btn-close-node-modal');
      const searchInput = document.getElementById('node-search-input');
      const categoryScroller = document.getElementById('node-categories-list');
      const previousCategory = document.getElementById('node-categories-prev');
      const nextCategory = document.getElementById('node-categories-next');

      if (categoryScroller) {
        categoryScroller.addEventListener('scroll', () => this.syncNodeCategoryScrollButtons(), { passive: true });
      }
      if (previousCategory) {
        previousCategory.addEventListener('click', () => {
          if (categoryScroller) categoryScroller.scrollBy({ left: -Math.max(240, categoryScroller.clientWidth * 0.75), behavior: 'smooth' });
        });
      }
      if (nextCategory) {
        nextCategory.addEventListener('click', () => {
          if (categoryScroller) categoryScroller.scrollBy({ left: Math.max(240, categoryScroller.clientWidth * 0.75), behavior: 'smooth' });
        });
      }
      window.addEventListener('resize', () => this.syncNodeCategoryScrollButtons());

      if (closeBtn) {
        closeBtn.addEventListener('click', () => this.closeAddNodeModal());
      }

      if (modal) {
        modal.addEventListener('click', (e) => {
          if (e.target === modal) this.closeAddNodeModal();
        });
      }

      if (searchInput) {
        searchInput.addEventListener('input', (e) => {
          const activeChip = document.querySelector('#node-categories-list .wf-tag-filter.active');
          const activeCat = activeChip ? activeChip.dataset.cat : 'all';
          this.renderModalGrid(e.target.value, activeCat);
        });
      }

      document.querySelectorAll('#node-categories-list .wf-tag-filter').forEach(chip => {
        chip.addEventListener('click', () => {
          document.querySelectorAll('#node-categories-list .wf-tag-filter').forEach(c => c.classList.remove('active'));
          document.querySelectorAll('#node-categories-list .wf-tag-filter').forEach(c => c.setAttribute('aria-pressed', 'false'));
          chip.classList.add('active');
          chip.setAttribute('aria-pressed', 'true');
          this.revealNodeCategory(chip);
          const category = chip.dataset.cat || 'all';
          if (category === 'all') {
            this.showAllNodes();
            return;
          }
          this.renderModalGrid(searchInput ? searchInput.value : '', category);
        });
      });
      this.syncNodeCategoryScrollButtons();
    }

    renderModalGrid(filterText = '', filterCat = 'all') {
      const grid = document.getElementById('modal-node-grid');
      if (!grid) return;
      grid.innerHTML = '';

      Object.values(this.definitions).forEach(def => {
        if (def.hidden) return;
        if (filterCat !== 'all') {
          if (filterCat === 'custom') {
            const isCustom = def.category === 'custom' || def.type.startsWith('custom_');
            if (!isCustom) return;
          } else if (def.category !== filterCat) {
            return;
          }
        }
        if (this.pendingConnect) {
          const compatible = (def.inputs || []).some(port => this.isPortTypeCompatible(this.pendingConnect.portType, port.type));
          if (!compatible) return;
        }
        if (filterText) {
          const query = filterText.toLowerCase();
          const matchTitle = def.title.toLowerCase().includes(query);
          const matchDesc = (def.description || '').toLowerCase().includes(query);
          const matchType = def.type.toLowerCase().includes(query);
          if (!matchTitle && !matchDesc && !matchType) return;
        }

        const cat = def.category || 'process';
        const card = document.createElement('div');
        card.className = `wf-library-card cat-${cat}`;
        card.innerHTML = `
          <div class="wf-card-top">
            <div class="wf-card-icon-box cat-${escapeHTML(cat)}">
              <span>${this.getCategoryIcon(cat)}</span>
            </div>
            <div>
              <h4 class="wf-card-title">${escapeHTML(def.title)}</h4>
            </div>
          </div>
          <p class="wf-card-desc">${escapeHTML(def.description || '')}</p>
          <div class="wf-card-bottom">
            <span class="wf-card-cat-badge">${escapeHTML(cat.toUpperCase())}</span>
            <span class="wf-card-add-text">+ Thêm vào Canvas</span>
          </div>
        `;

        card.addEventListener('click', () => {
          let posX = 200, posY = 200;
          if (this.pendingConnect) {
            // Position near the source node
            const parentNode = this.graph.nodes.find(n => n.id === this.pendingConnect.fromNode);
            if (parentNode) {
              posX = (parentNode.x || 100) + 320;
              posY = (parentNode.y || 100);
            }
          } else {
            // Place near screen center
            const containerRect = this.container.getBoundingClientRect();
            posX = (-this.panX + containerRect.width / 2 - 135) / this.scale;
            posY = (-this.panY + containerRect.height / 2 - 60) / this.scale;
          }

          this.addNode(def.type, posX, posY);
          this.closeAddNodeModal();
        });

        grid.appendChild(card);
      });

      if (!grid.children.length) {
        const empty = document.createElement('div');
        empty.className = 'wf-library-empty';
        empty.setAttribute('role', 'status');
        empty.setAttribute('aria-live', 'polite');
        const hasFilters = Boolean(filterText.trim()) || filterCat !== 'all' || Boolean(this.pendingConnect);
        const icon = document.createElement('div');
        icon.className = 'wf-library-empty-icon';
        icon.setAttribute('aria-hidden', 'true');
        icon.innerHTML = this.pendingConnect
          ? '<svg viewBox="0 0 24 24" width="26" height="26" fill="none" stroke="currentColor" stroke-width="1.8"><path d="M8 7h8v10H8z"/><path d="M3 12h4m10 0h4M5 10l2 2-2 2m14-4-2 2 2 2"/></svg>'
          : '<svg viewBox="0 0 24 24" width="26" height="26" fill="none" stroke="currentColor" stroke-width="1.8"><circle cx="10.8" cy="10.8" r="6.8"/><path d="m16 16 4.5 4.5"/><path d="M8.5 10.8h4.6"/></svg>';
        const title = document.createElement('h4');
        const description = document.createElement('p');
        const action = document.createElement('button');
        action.type = 'button';
        action.className = 'wf-library-empty-action';

        if (this.pendingConnect) {
          title.textContent = 'Chưa tìm thấy node tương thích';
          description.textContent = 'Bỏ lọc kết nối để xem toàn bộ thư viện node.';
          action.textContent = 'Xem tất cả nodes';
        } else if (hasFilters) {
          title.textContent = 'Không tìm thấy node';
          description.textContent = 'Thử từ khóa khác hoặc mở lại toàn bộ thư viện node.';
          action.textContent = 'Xem tất cả nodes';
        } else {
          title.textContent = 'Thư viện node đang trống';
          description.textContent = 'Hiện chưa có node nào sẵn sàng để thêm vào workflow.';
          action.hidden = true;
        }

        if (!action.hidden) {
          action.addEventListener('click', (event) => {
            event.preventDefault();
            this.showAllNodes();
            document.getElementById('node-search-input')?.focus();
          });
        }

        empty.append(icon, title, description, action);
        grid.appendChild(empty);
      }
    }

    bindGraphsModal() {
      const modal = document.getElementById('wf-graphs-modal');
      const closeBtn = document.getElementById('btn-close-graphs-modal');
      const searchInput = document.getElementById('graphs-search-input');
      const createBtn = document.getElementById('btn-modal-create-graph');

      if (closeBtn) {
        closeBtn.addEventListener('click', () => this.closeGraphsModal());
      }

      if (modal) {
        modal.addEventListener('click', (e) => {
          if (e.target === modal) this.closeGraphsModal();
        });
      }

      if (createBtn) {
        createBtn.addEventListener('click', async () => {
          if (await this.createNewGraph()) this.closeGraphsModal();
        });
      }

      if (searchInput) {
        searchInput.addEventListener('input', (e) => {
          const activeChip = document.querySelector('#graphs-filter-tabs .wf-tag-filter.active');
          const activeFilter = activeChip ? activeChip.dataset.filter : 'all';
          this.renderGraphsModalGrid(e.target.value, activeFilter);
        });
      }

      document.querySelectorAll('#graphs-filter-tabs .wf-tag-filter').forEach(chip => {
        chip.addEventListener('click', () => {
          document.querySelectorAll('#graphs-filter-tabs .wf-tag-filter').forEach(c => c.classList.remove('active'));
          chip.classList.add('active');
          const query = searchInput ? searchInput.value : '';
          this.renderGraphsModalGrid(query, chip.dataset.filter);
        });
      });
    }

    openGraphsModal() {
      const modal = document.getElementById('wf-graphs-modal');
      if (modal) {
        modal.style.display = 'flex';
        const searchInput = document.getElementById('graphs-search-input');
        if (searchInput) searchInput.value = '';
        this.renderGraphsModalGrid('', 'all');
      }
    }

    closeGraphsModal() {
      const modal = document.getElementById('wf-graphs-modal');
      if (modal) modal.style.display = 'none';
    }

    renderGraphsModalGrid(query = '', filter = 'all') {
      const grid = document.getElementById('modal-graphs-grid');
      if (!grid) return;
      grid.innerHTML = '';

      const graphs = this.savedGraphs || [];
      const q = query.toLowerCase().trim();

      // Cập nhật số lượng đếm trên các tab bộ lọc
      const total = graphs.length;
      const presetsCount = graphs.filter(g => g.is_preset).length;
      const customCount = graphs.filter(g => !g.is_preset).length;

      const countAll = document.getElementById('count-all');
      if (countAll) countAll.innerText = `(${total})`;
      const countCustom = document.getElementById('count-custom');
      if (countCustom) countCustom.innerText = `(${customCount})`;
      const countPreset = document.getElementById('count-preset');
      if (countPreset) countPreset.innerText = `(${presetsCount})`;

      let matchCount = 0;

      graphs.forEach(g => {
        if (filter === 'presets' || filter === 'preset') {
          if (!g.is_preset) return;
        } else if (filter === 'custom') {
          if (g.is_preset) return;
        } else if (filter !== 'all') {
          if (g.category !== filter) return;
        }

        if (q) {
          const matchName = (g.name || '').toLowerCase().includes(q);
          const matchDesc = (g.description || '').toLowerCase().includes(q);
          const matchCat = (g.category || '').toLowerCase().includes(q);
          if (!matchName && !matchDesc && !matchCat) return;
        }

        matchCount++;

        const isActive = this.graph && this.graph.id === g.id;
        const card = document.createElement('div');
        card.className = `wf-graph-card ${isActive ? 'is-active' : ''}`;

        let catIcon = '⚡';
        if (g.category === 'viral') catIcon = '🕺';
        else if (g.category === 'fashion') catIcon = '👗';
        else if (g.category === 'affiliate') catIcon = '🛍️';
        else if (g.category === 'custom') catIcon = '🧠';

        let catTag = 'tag-custom';
        let catLabel = '💾 Luồng của bạn';
        if (g.is_preset) {
          catTag = 'tag-preset';
          catLabel = '✨ Mẫu hệ thống';
        } else if (g.category === 'fashion') {
          catTag = 'tag-fashion';
          catLabel = '👗 Thời trang';
        }

        card.innerHTML = `
          <div>
            ${isActive ? `<div class="wf-graph-active-badge">● Đang mở trên Canvas</div>` : ''}
            <div class="wf-graph-card-header">
              <div class="wf-graph-card-title-group">
                <span class="wf-graph-card-icon">${catIcon}</span>
                <span class="wf-graph-card-title">${escapeHTML(g.name)}</span>
              </div>
              <span class="wf-graph-tag ${catTag}">${catLabel}</span>
            </div>
            <p class="wf-graph-card-desc">${escapeHTML(g.description || 'Workflow tự động hóa sản xuất video AI Creator OS')}</p>
          </div>
          <div class="wf-graph-card-meta">
            <div class="wf-graph-card-meta-info">
              <span>⚡ ${Number(g.node_count) || 0} nodes</span>
              <span>•</span>
              <span>🔗 ${Number(g.edge_count) || 0} dây nối</span>
            </div>
            <div class="wf-graph-card-actions">
              ${!g.is_preset ? `
                <button type="button" class="wf-btn-del-graph btn-del-graph" title="Xóa vĩnh viễn workflow đã lưu này">
                  🗑️ Xóa
                </button>
              ` : ''}
              <button type="button" class="wf-btn-dup-graph btn-dup-graph" title="Tạo bản sao riêng để bạn tự do chỉnh sửa và xóa">
                📑 Nhân bản
              </button>
              <button type="button" class="wf-btn-open-graph btn-open-graph">
                ${isActive ? '✓ Đang xem' : 'Mở'}
              </button>
            </div>
          </div>
        `;

        // Sự kiện mở workflow
        const openBtn = card.querySelector('.btn-open-graph');
        if (openBtn) {
          openBtn.addEventListener('click', async (e) => {
            e.stopPropagation();
            await this.loadGraphById(g.id);
          });
        }

        // Sự kiện nhân bản
        const dupBtn = card.querySelector('.btn-dup-graph');
        if (dupBtn) {
          dupBtn.addEventListener('click', async (e) => {
            e.stopPropagation();
            await this.duplicateGraph(g.id, g.name);
          });
        }

        // Sự kiện xóa workflow đã lưu
        const delBtn = card.querySelector('.btn-del-graph');
        if (delBtn) {
          delBtn.addEventListener('click', async (e) => {
            e.stopPropagation();
            await this.deleteGraph(g.id, g.name);
          });
        }

        card.addEventListener('click', async () => {
          await this.loadGraphById(g.id);
        });

        grid.appendChild(card);
      });

      // Trạng thái trống nếu không có kết quả tìm kiếm
      if (matchCount === 0) {
        grid.innerHTML = `
          <div style="grid-column: 1 / -1; text-align: center; padding: 48px 20px; color: var(--muted);">
            <div style="font-size: 38px; margin-bottom: 12px;">📂</div>
            <div style="font-weight: 700; font-size: 16px; color: var(--text);">Không tìm thấy workflow phù hợp</div>
            <p style="font-size: 13px; margin: 6px 0 18px; color: var(--muted);">Bạn có thể thử tìm với từ khóa khác hoặc bấm tạo một luồng workflow mới.</p>
            <button type="button" class="wf-modal-create-btn" style="margin: 0 auto;" onclick="document.getElementById('btn-modal-create-graph').click()">
              <svg viewBox="0 0 24 24" width="15" height="15" stroke="currentColor" stroke-width="2.5" fill="none"><line x1="12" y1="5" x2="12" y2="19"/><line x1="5" y1="12" x2="19" y2="12"/></svg>
              <span>Tạo mới</span>
            </button>
          </div>
        `;
      }
    }

    async duplicateGraph(id, name) {
      try {
        const resp = await fetch(`/api/flow/graphs/${id}`);
        const data = await resp.json();
        if (!data.ok || !data.graph) {
          window.showAppNotice('Không thể đọc dữ liệu workflow để nhân bản');
          return;
        }

        const cloned = JSON.parse(JSON.stringify(data.graph));
        cloned.id = `flow_${Date.now()}`;
        cloned.name = `${name} (Bản sao)`;
        cloned.is_preset = false;
        cloned.created_at = new Date().toISOString();
        cloned.updated_at = new Date().toISOString();

        const saveResp = await fetch('/api/flow/graphs', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify(cloned)
        });
        const saveRes = await saveResp.json();
        if (saveRes.ok) {
          await this.refreshGraphsList();
          this.renderGraphsModalGrid();
          await this.loadGraphById(cloned.id);
          window.showAppNotice(`✓ Đã nhân bản thành công: "${cloned.name}". Đây là bản lưu riêng của bạn, bạn có thể tự do chỉnh sửa hoặc xóa!`);
          this.closeGraphsModal();
        } else {
          window.showAppNotice('Lỗi nhân bản: ' + (saveRes.error || ''));
        }
      } catch (err) {
        window.showAppNotice('Lỗi kết nối máy chủ: ' + err.message);
      }
    }

    async loadGraphById(id, force = false) {
      if (!force && !await this.confirmDiscardGraphChanges()) return false;
      try {
        const resp = await fetch('/api/flow/graphs/' + encodeURIComponent(id));
        const data = await resp.json();
        if (data.ok && data.graph) {
          this.workflowContractRequests.clear();
          this.workflowContractLoading.clear();
          this.graph = data.graph;
          if (!this.graph.nodes) this.graph.nodes = [];
          if (!this.graph.edges) this.graph.edges = [];
          this.graphPersisted = true;
          this.savedGraphFingerprint = this.graphFingerprint();
          this.render();
          requestAnimationFrame(() => this.fitCanvas());
          this.deselectAll();
          this.syncGraphHeader();
          const switcher = document.getElementById('wf-switcher-select');
          if (switcher) switcher.value = id;
          window.history.pushState(null, '', '/workflow?preset=' + encodeURIComponent(id));
          this.closeGraphsModal();
          return true;
        }
        window.showAppNotice('Không thể mở workflow: ' + (data.error || ''));
      } catch (err) {
        console.error('Lỗi tải workflow:', err);
        window.showAppNotice('Lỗi kết nối máy chủ: ' + err.message);
      }
      return false;
    }

    async createNewGraph() {
      if (!await this.confirmDiscardGraphChanges()) return false;
      this.workflowContractRequests.clear();
      this.workflowContractLoading.clear();
      const newId = 'flow_' + Date.now();
      this.graph = {
        id: newId,
        name: 'Workflow mới tùy chỉnh',
        description: 'Tạo bởi người dùng',
        category: 'custom',
        is_preset: false,
        nodes: [],
        edges: []
      };
      this.graphPersisted = false;
      this.savedGraphFingerprint = this.graphFingerprint();
      this.render();
      this.deselectAll();
      this.syncGraphHeader();
      window.history.pushState(null, '', '/workflow?preset=' + newId);
      this.openAddNodeModal();
      return true;
    }

    async deleteGraph(id, name) {
      if (!await window.showAppConfirm(`Bạn có chắc muốn xóa vĩnh viễn workflow "${name}"? Thao tác này không thể hoàn tác.`, { title: 'Xóa workflow', confirmText: 'Xóa vĩnh viễn', danger: true })) {
        return;
      }
      try {
        const resp = await fetch(`/api/flow/graphs/${id}`, { method: 'DELETE' });
        const data = await resp.json();
        if (data.ok) {
          this.savedGraphs = this.savedGraphs.filter(g => g.id !== id);
          await this.refreshGraphsList();
          this.renderGraphsModalGrid();
          if (this.graph && this.graph.id === id) {
            this.loadGraphById('preset_viral_dance', true);
          }
          window.showAppNotice(`✓ Đã xóa thành công workflow "${name}"!`);
        } else {
          window.showAppNotice('Không thể xóa workflow: ' + (data.error || ''));
        }
      } catch (err) {
        window.showAppNotice('Lỗi kết nối máy chủ: ' + err.message);
      }
    }

    async refreshGraphsList() {
      try {
        const resp = await fetch('/api/flow/graphs');
        const data = await resp.json();
        if (data.ok && Array.isArray(data.graphs)) {
          this.savedGraphs = data.graphs;

          // Update count label
          const countLabel = document.getElementById('wf-saved-count-label');
          if (countLabel) {
            countLabel.innerText = `Danh sách (${data.graphs.length})`;
          }

          // Update select dropdown
          const switcher = document.getElementById('wf-switcher-select');
          if (switcher) {
            switcher.innerHTML = '';
            if (!data.graphs || data.graphs.length === 0) {
              switcher.style.display = 'none';
            } else {
              switcher.style.display = '';
              const presetGroup = document.createElement('optgroup');
              presetGroup.label = '✨ Mẫu workflow có sẵn';

              const userGroup = document.createElement('optgroup');
              userGroup.label = '💾 Workflow bạn đã lưu';

              data.graphs.forEach(g => {
                const opt = document.createElement('option');
                opt.value = g.id;
                opt.innerText = g.name;
                if (this.graph && this.graph.id === g.id) opt.selected = true;

                if (g.is_preset) {
                  presetGroup.appendChild(opt);
                } else {
                  userGroup.appendChild(opt);
                }
              });

              if (presetGroup.children.length > 0) switcher.appendChild(presetGroup);
              if (userGroup.children.length > 0) switcher.appendChild(userGroup);
            }
          }
        }
      } catch (e) {
        console.error('Không thể cập nhật danh sách workflow', e);
      }
    }

    isPresetWorkflow(id) {
      if (!id) return false;
      if (this.graph && this.graph.id === id && this.graph.is_preset) return true;
      if (String(id).startsWith('preset_')) return true;
      if (Array.isArray(this.savedGraphs)) {
        const found = this.savedGraphs.find(g => g.id === id);
        if (found && found.is_preset) return true;
      }
      return false;
    }

  }

  function initCanvas() {
    if (!window.flowCanvas) {
      window.flowCanvas = new FlowCanvas();
    }
  }

  if (document.readyState === 'loading') {
    document.addEventListener('DOMContentLoaded', initCanvas);
  } else {
    initCanvas();
  }
})();
