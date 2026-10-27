// FlowCanvas - Interactive Node Graph Workflow Canvas (n8n & ComfyUI style)
(function () {
  'use strict';

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
      this.pollInterval = null;
      this.savedGraphs = window.__SAVED_GRAPHS__ || [];

      this.initEvents();
      this.render();
      this.bindToolbar();
      this.bindInspector();
      this.bindModal();
      this.bindGraphsModal();
      this.updateZoomDisplay();
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
        if ((e.key === 'Delete' || e.key === 'Backspace') && this.selectedNodeId) {
          const activeTag = document.activeElement ? document.activeElement.tagName.toLowerCase() : '';
          if (activeTag === 'input' || activeTag === 'textarea' || activeTag === 'select') return;
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
      const def = this.definitions[node.type] || {
        type: node.type,
        title: node.title || node.type,
        category: node.category || 'process',
        inputs: [],
        outputs: []
      };

      const cat = node.category || def.category || 'process';

      const nodeEl = document.createElement('div');
      nodeEl.className = `wf-node cat-${cat}`;
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
      header.innerHTML = `
        <div class="wf-node-icon cat-${cat}">
          <span>${this.getCategoryIcon(cat)}</span>
        </div>
        <div class="wf-node-info">
          <div class="wf-node-title" title="${node.title || def.title || node.type}">
            ${node.title || def.title || node.type}
          </div>
          <div class="wf-node-subtitle" title="${def.title || node.type}">
            ${def.title || node.type}
          </div>
        </div>
        <div class="wf-node-actions">
          <button type="button" class="wf-node-del-btn" title="Xóa node">&times;</button>
        </div>
      `;

      // Delete Node
      header.querySelector('.wf-node-del-btn').addEventListener('click', (e) => {
        e.stopPropagation();
        this.deleteNode(node.id);
      });

      // Drag Node (Entire node card is smoothly draggable)
      let startClientX = 0, startClientY = 0;
      let startNodeX = 0, startNodeY = 0;
      let isDraggingNode = false;

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

        this.selectNode(node.id);

        isDraggingNode = true;
        startClientX = e.clientX;
        startClientY = e.clientY;
        startNodeX = typeof node.x === 'number' ? node.x : (parseFloat(nodeEl.style.left) || 100);
        startNodeY = typeof node.y === 'number' ? node.y : (parseFloat(nodeEl.style.top) || 100);

        nodeEl.classList.add('is-dragging');
        document.body.style.userSelect = 'none';

        const onMouseMove = (ev) => {
          if (!isDraggingNode) return;
          const dx = (ev.clientX - startClientX) / this.scale;
          const dy = (ev.clientY - startClientY) / this.scale;

          node.x = Math.round(startNodeX + dx);
          node.y = Math.round(startNodeY + dy);

          nodeEl.style.left = `${node.x}px`;
          nodeEl.style.top = `${node.y}px`;

          this.renderWires();
        };

        const onMouseUp = () => {
          if (isDraggingNode) {
            isDraggingNode = false;
            nodeEl.classList.remove('is-dragging');
            document.body.style.userSelect = '';
          }
          window.removeEventListener('mousemove', onMouseMove);
          window.removeEventListener('mouseup', onMouseUp);
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

      const inputs = node.inputs || def.inputs || [];
      inputs.forEach(port => {
        inputsCol.appendChild(this.createPortElement(node.id, port, false));
      });

      const outputs = node.outputs || def.outputs || [];
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
      footer.innerHTML = this.renderNodeStatus(node.state || { status: 'idle' });
      nodeEl.appendChild(footer);

      // Click anywhere on node to select
      nodeEl.addEventListener('click', (e) => {
        e.stopPropagation();
        this.selectNode(node.id);
      });

      this.nodesLayer.appendChild(nodeEl);
    }

    createPortElement(nodeId, port, isOutput) {
      const portEl = document.createElement('div');
      portEl.className = `wf-port ${isOutput ? 'wf-port-output' : 'wf-port-input'} port-type-${port.type || 'any'}`;
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
        dot.innerHTML = '<span class="wf-port-plus">+</span>';
        dot.title = `Kéo để nối dây hoặc bấm (+) để thêm bước tiếp theo (${port.name})`;
        portEl.appendChild(label);
        portEl.appendChild(dot);

        // Click / Drag handle logic for output port (+)
        let isDrag = false;
        let startClientX = 0, startClientY = 0;

        dot.addEventListener('mousedown', (e) => {
          if (e.button !== 0) return;
          e.stopPropagation();

          isDrag = false;
          startClientX = e.clientX;
          startClientY = e.clientY;

          const rect = dot.getBoundingClientRect();
          const containerRect = this.container.getBoundingClientRect();
          const startX = (rect.left + rect.width / 2 - containerRect.left - this.panX) / this.scale;
          const startY = (rect.top + rect.height / 2 - containerRect.top - this.panY) / this.scale;

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
        dot.title = `${port.name} (${port.type})`;
        portEl.appendChild(dot);
        portEl.appendChild(label);

        // Drop wire connection on input dot
        dot.addEventListener('mouseup', (e) => {
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

      const previewContainer = document.createElement('div');
      previewContainer.className = `wf-node-preview ${config.isCutout ? 'preview-cutout' : ''}`;
      previewContainer.id = `node-preview-${node.id}`;

      // Badge
      const badge = document.createElement('div');
      badge.className = 'wf-preview-badge';
      badge.innerHTML = config.badge;
      previewContainer.appendChild(badge);

      // Media element
      if (config.type === 'video') {
        const video = document.createElement('video');
        video.src = config.url;
        video.muted = true;
        video.loop = true;
        video.playsInline = true;
        video.preload = 'metadata';
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
        img.src = config.url;
        img.alt = node.title || 'Preview';
        img.loading = 'lazy';
        previewContainer.appendChild(img);
      } else if (config.type === 'audio') {
        const audioBox = document.createElement('div');
        audioBox.className = 'wf-preview-audio-box';
        audioBox.innerHTML = `
          <div class="wf-preview-waveform">
            <span style="height:40%"></span><span style="height:80%"></span><span style="height:60%"></span>
            <span style="height:100%"></span><span style="height:70%"></span><span style="height:35%"></span>
            <span style="height:90%"></span><span style="height:55%"></span><span style="height:75%"></span>
          </div>
          <div class="wf-preview-audio-title">${node.params?.audio_title || 'Âm thanh thịnh hành (128 BPM)'}</div>
        `;
        previewContainer.appendChild(audioBox);
      }

      // Quick actions toolbar
      const actions = document.createElement('div');
      actions.className = 'wf-preview-actions';

      // 1. Zoom / Lightbox button
      const zoomBtn = document.createElement('button');
      zoomBtn.type = 'button';
      zoomBtn.className = 'wf-preview-btn';
      zoomBtn.title = 'Phóng to xem chi tiết';
      zoomBtn.innerHTML = '🔍 Xem';
      zoomBtn.addEventListener('click', (e) => {
        e.stopPropagation();
        this.openLightbox(node.title || 'Preview', config.url, config.type);
      });
      actions.appendChild(zoomBtn);

      // 2. Upload button (if node supports uploading input asset)
      if (config.canUpload) {
        const uploadBtn = document.createElement('button');
        uploadBtn.type = 'button';
        uploadBtn.className = 'wf-preview-btn';
        uploadBtn.title = 'Tải ảnh/video mới lên node này';
        uploadBtn.innerHTML = '📤 Tải lên';

        const fileInput = document.createElement('input');
        fileInput.type = 'file';
        fileInput.accept = config.type === 'video' ? 'video/mp4,video/*' : 'image/*';
        fileInput.style.display = 'none';

        fileInput.addEventListener('change', (ev) => {
          const file = ev.target.files && ev.target.files[0];
          if (!file) return;

          const reader = new FileReader();
          reader.onload = (re) => {
            const dataUrl = re.target.result;
            if (!node.params) node.params = {};
            node.params.preview_url = dataUrl;
            if (config.type === 'video') {
              node.params.video_path = file.name;
            } else {
              node.params.model_path = file.name;
              node.params.product_image = file.name;
            }
            this.updateNodePreview(node.id, dataUrl);
            this.propagatePreviewUpdate(node.id, dataUrl);
          };
          reader.readAsDataURL(file);
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
        this.openLightbox(node.title || 'Preview', config.url, config.type);
      });

      return previewContainer;
    }

    getNodePreviewConfig(node) {
      const type = node.type;
      const params = node.params || {};

      switch (type) {
        case 'character_input':
          return {
            type: 'image',
            url: params.preview_url || '/static/presets/char_01.jpg',
            badge: '👤 Nhân vật mẫu',
            canUpload: true,
            isCutout: false
          };

        case 'product_input':
          return {
            type: 'image',
            url: params.preview_url || '/static/presets/char_04.jpg',
            badge: '🛍️ Sản phẩm mẫu',
            canUpload: true,
            isCutout: false
          };

        case 'product_extractor':
        case 'background_remover':
          return {
            type: 'cutout',
            url: params.preview_url || this.getConnectedInputPreview(node.id) || '/static/presets/char_04.jpg',
            badge: '✂️ Đã tách nền PNG',
            canUpload: false,
            isCutout: true
          };

        case 'image_composite':
          return {
            type: 'image',
            url: params.preview_url || '/static/presets/char_02.jpg',
            badge: '✨ Ghép phối cảnh AI',
            canUpload: false,
            isCutout: false
          };

        case 'video_source':
          return {
            type: 'video',
            url: params.preview_url || '/static/sample_916.mp4',
            badge: '🎬 Video TikTok Radar (13.8s)',
            canUpload: true,
            isCutout: false
          };

        case 'dance_motion_extractor':
          return {
            type: 'image',
            url: '/static/agents/dancer.jpg',
            badge: '🕺 Vũ đạo 128 BPM (18 Poses)',
            canUpload: false,
            isCutout: false
          };

        case 'dance_retarget':
          return {
            type: 'image',
            url: params.preview_url || this.getConnectedInputPreview(node.id) || '/static/presets/char_01.jpg',
            badge: '🎭 Khớp khung xương',
            canUpload: false,
            isCutout: false
          };

        case 'virtual_tryon':
          return {
            type: 'image',
            url: params.preview_url || this.getConnectedInputPreview(node.id) || '/static/presets/pintest-01.jpg',
            badge: '👗 Thay trang phục AI',
            canUpload: false,
            isCutout: false
          };

        case 'camera_angles_director':
          return {
            type: 'image',
            url: params.preview_url || this.getConnectedInputPreview(node.id) || '/static/presets/pintest-01.jpg',
            badge: '🎬 3 Góc quay điện ảnh',
            canUpload: false,
            isCutout: false
          };

        case 'google_flow_video':
          return {
            type: 'video',
            url: '/static/sample_916.mp4',
            badge: '✨ Google Flow Veo 2',
            canUpload: false,
            isCutout: false
          };

        case 'muse_ai_video':
          return {
            type: 'video',
            url: '/static/sample_916.mp4',
            badge: '⚡ Muse AI Video',
            canUpload: false,
            isCutout: false
          };

        case 'custom_web_ai':
          return {
            type: 'video',
            url: '/static/sample_916.mp4',
            badge: '🧠 Web AI Video',
            canUpload: false,
            isCutout: false
          };

        case 'video_upscale':
          return {
            type: 'video',
            url: '/static/sample_916.mp4',
            badge: '💎 4K 60FPS UHD Enhanced',
            canUpload: false,
            isCutout: false
          };

        case 'video_output':
          return {
            type: 'video',
            url: '/static/sample_916.mp4',
            badge: '🎉 Video hoàn thiện 9:16',
            canUpload: false,
            isCutout: false
          };

        case 'image_output':
          return {
            type: 'image',
            url: params.preview_url || this.getConnectedInputPreview(node.id) || '/static/presets/char_01.jpg',
            badge: '🖼️ Ảnh hoàn thiện',
            canUpload: false,
            isCutout: false
          };

        case 'audio_source':
        case 'audio_merge':
          return {
            type: 'audio',
            url: '/static/preview.mp3',
            badge: '🎵 Âm thanh thịnh hành',
            canUpload: false,
            isCutout: false
          };

        default:
          return null;
      }
    }

    getConnectedInputPreview(nodeId) {
      if (!this.graph.edges) return null;
      const incomingEdge = this.graph.edges.find(e => e.to_node === nodeId);
      if (!incomingEdge) return null;
      const sourceNode = this.graph.nodes.find(n => n.id === incomingEdge.from_node);
      if (!sourceNode) return null;
      return sourceNode.params?.preview_url || null;
    }

    propagatePreviewUpdate(nodeId, newUrl) {
      if (!this.graph.edges) return;
      const outgoingEdges = this.graph.edges.filter(e => e.from_node === nodeId);
      outgoingEdges.forEach(edge => {
        const targetNode = this.graph.nodes.find(n => n.id === edge.to_node);
        if (targetNode) {
          if (!targetNode.params) targetNode.params = {};
          targetNode.params.preview_url = newUrl;
          this.updateNodePreview(targetNode.id, newUrl);
          this.propagatePreviewUpdate(targetNode.id, newUrl);
        }
      });
    }

    updateNodePreview(nodeId, newUrl) {
      const previewEl = document.getElementById(`node-preview-${nodeId}`);
      if (!previewEl) return;
      const img = previewEl.querySelector('img');
      if (img) img.src = newUrl;
      const video = previewEl.querySelector('video');
      if (video) video.src = newUrl;
    }

    openLightbox(title, url, type) {
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
      this.svgLayer.innerHTML = '';
      if (!this.graph.edges) return;

      this.graph.edges.forEach(edge => {
        const fromDot = this.findPortDot(edge.from_node, edge.from_port, true);
        const toDot = this.findPortDot(edge.to_node, edge.to_port, false);
        if (!fromDot || !toDot) return;

        const p1 = this.getPortCoordinates(fromDot);
        const p2 = this.getPortCoordinates(toDot);
        const d = this.calculateBezier(p1.x, p1.y, p2.x, p2.y);

        const fromNode = this.graph.nodes.find(n => n.id === edge.from_node);
        const toNode = this.graph.nodes.find(n => n.id === edge.to_node);
        const portType = fromDot.parentElement ? fromDot.parentElement.dataset.type : 'any';

        const isRunning = (fromNode?.state?.status === 'running') || (toNode?.state?.status === 'running');

        const group = document.createElementNS('http://www.w3.org/2000/svg', 'g');
        group.setAttribute('class', `flow-wire-group ${isRunning ? 'active-flowing' : ''}`);
        group.dataset.edgeId = edge.id;

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
        hitPath.addEventListener('click', (e) => {
          e.stopPropagation();
          if (confirm('Bạn có muốn xóa kết nối dây này?')) {
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
      const containerRect = this.container.getBoundingClientRect();
      const curX = (clientX - containerRect.left - this.panX) / this.scale;
      const curY = (clientY - containerRect.top - this.panY) / this.scale;

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
      const dx = Math.max(50, Math.abs(x2 - x1) * 0.55);
      return `M ${x1} ${y1} C ${x1 + dx} ${y1}, ${x2 - dx} ${y2}, ${x2} ${y2}`;
    }

    findPortDot(nodeId, portId, isOutput) {
      const selector = `.wf-port[data-node-id="${nodeId}"][data-port-id="${portId}"][data-is-output="${isOutput ? '1' : '0'}"] .wf-port-dot`;
      return this.nodesLayer.querySelector(selector);
    }

    getPortCoordinates(dotEl) {
      const rect = dotEl.getBoundingClientRect();
      const containerRect = this.container.getBoundingClientRect();
      return {
        x: (rect.left + rect.width / 2 - containerRect.left - this.panX) / this.scale,
        y: (rect.top + rect.height / 2 - containerRect.top - this.panY) / this.scale
      };
    }

    addEdge(fromNode, fromPort, toNode, toPort) {
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
    }

    deleteEdge(edgeId) {
      this.graph.edges = this.graph.edges.filter(e => e.id !== edgeId);
      this.renderWires();
    }

    deleteNode(nodeId) {
      this.graph.nodes = this.graph.nodes.filter(n => n.id !== nodeId);
      this.graph.edges = this.graph.edges.filter(e => e.from_node !== nodeId && e.to_node !== nodeId);
      if (this.selectedNodeId === nodeId) {
        this.deselectAll();
      }
      this.render();
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
        const matchingInput = (newNode.inputs || []).find(inp => inp.type === this.pendingConnect.portType) || (newNode.inputs || [])[0];
        if (matchingInput) {
          this.addEdge(fromNode, fromPort, id, matchingInput.id);
        }
        this.pendingConnect = null;
      }

      this.render();
      this.selectNode(id);
    }

    selectNode(nodeId) {
      this.selectedNodeId = nodeId;
      this.nodesLayer.querySelectorAll('.wf-node').forEach(el => {
        el.classList.toggle('selected', el.id === `node-${nodeId}`);
      });
      this.openInspector();
      this.updateInspector();
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

    renderNodeStatus(state) {
      if (!state || state.status === 'idle') {
        return `<span class="wf-status-badge wf-status-idle">Chờ thực thi</span>`;
      }
      if (state.status === 'running') {
        return `<span class="wf-status-badge wf-status-running"><span class="wf-spinner"></span> ⚡ Đang thực thi (${state.progress || 10}%)...</span>`;
      }
      if (state.status === 'completed') {
        let preview = '';
        if (state.output_preview) {
          const fn = this.getFilename(state.output_preview);
          if (fn.endsWith('.mp4') || fn.endsWith('.webm')) {
            preview = `<div class="wf-node-preview-thumb"><video src="/media/${fn}" autoplay muted loop playsinline></video></div>`;
          } else {
            preview = `<div class="wf-node-preview-thumb"><img src="/media/${fn}" alt="Preview"></div>`;
          }
        }
        return `<div class="wf-status-badge wf-status-completed">✓ Hoàn tất 100%</div>${preview}`;
      }
      if (state.status === 'failed') {
        return `<span class="wf-status-badge wf-status-failed" title="${state.error || ''}">⚠️ Lỗi: ${state.error ? state.error.slice(0, 26) : 'Thất bại'}</span>`;
      }
      return '';
    }

    getFilename(path) {
      if (!path) return '';
      return path.split('/').pop();
    }

    getCategoryIcon(cat) {
      switch (cat) {
        case 'input': return '📥';
        case 'process': return '⚙️';
        case 'ai': return '🧠';
        case 'output': return '📤';
        default: return '📦';
      }
    }

    bindToolbar() {
      // Workflow switcher dropdown
      const switcherSelect = document.getElementById('wf-switcher-select');
      if (switcherSelect) {
        switcherSelect.addEventListener('change', (e) => {
          this.loadGraphById(e.target.value);
        });
      }

      // Legacy preset select fallback
      const presetSelect = document.getElementById('wf-preset-select');
      if (presetSelect) {
        presetSelect.addEventListener('change', (e) => {
          this.loadGraphById(e.target.value);
        });
      }

      // Open graphs list modal
      const btnOpenGraphs = document.getElementById('btn-open-graphs-modal');
      if (btnOpenGraphs) {
        btnOpenGraphs.addEventListener('click', () => {
          this.openGraphsModal();
        });
      }

      // Create new workflow button
      const btnNew = document.getElementById('btn-new-graph');
      if (btnNew) {
        btnNew.addEventListener('click', () => {
          if (confirm('Tạo một workflow mới trống? Các thay đổi chưa lưu trên luồng hiện tại có thể bị mất.')) {
            this.createNewGraph();
          }
        });
      }

      // Add Node button
      const btnAdd = document.getElementById('btn-add-node');
      if (btnAdd) {
        btnAdd.addEventListener('click', () => {
          this.pendingConnect = null;
          this.openAddNodeModal();
        });
      }

      // Save Graph button
      const btnSave = document.getElementById('btn-save-graph');
      if (btnSave) {
        btnSave.addEventListener('click', async () => {
          let name = this.graph.name;
          if (this.graph.is_preset) {
            const newName = prompt('Đây là mẫu preset có sẵn. Vui lòng đặt tên cho bản lưu riêng của bạn:', `${this.graph.name} (Bản sao)`);
            if (!newName) return;
            name = newName;
            this.graph.id = `flow_${Date.now()}`;
            this.graph.name = newName;
            this.graph.is_preset = false;
          }

          try {
            const resp = await fetch('/api/flow/graphs', {
              method: 'POST',
              headers: { 'Content-Type': 'application/json' },
              body: JSON.stringify(this.graph)
            });
            const data = await resp.json();
            if (data.ok) {
              const activeName = document.getElementById('wf-active-name');
              if (activeName) activeName.innerText = this.graph.name;
              window.history.pushState(null, '', `/workflow?preset=${encodeURIComponent(this.graph.id)}`);
              alert(`✓ Đã lưu workflow "${this.graph.name}" thành công!`);
              await this.refreshGraphsList();
            } else {
              alert('Lỗi lưu workflow: ' + (data.error || ''));
            }
          } catch (err) {
            alert('Lỗi kết nối máy chủ: ' + err.message);
          }
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
    }

    async runFlow() {
      const runBtn = document.getElementById('btn-run-flow');
      const cancelBtn = document.getElementById('btn-cancel-flow');
      const statusPill = this.statusPill;

      if (runBtn) runBtn.disabled = true;
      if (cancelBtn) cancelBtn.style.display = 'inline-flex';
      if (statusPill) {
        statusPill.className = 'wf-status-pill status-running';
        statusPill.innerText = '⚡ Đang chạy workflow...';
      }

      // Reset visual node states to queued
      this.graph.nodes.forEach(n => {
        n.state = { status: 'queued', progress: 0 };
        const footer = document.getElementById(`node-footer-${n.id}`);
        if (footer) footer.innerHTML = this.renderNodeStatus(n.state);
      });

      // Switch inspector to logs tab
      const logTabBtn = document.querySelector('.wf-tab[data-tab="run-logs"]');
      if (logTabBtn) logTabBtn.click();
      this.openInspector();

      this.clearLogs();
      this.appendLog('[Khởi chạy] Đang kiểm tra cấu hình đồ thị Workflow...');

      try {
        const resp = await fetch('/api/flow/execute', {
          method: 'POST',
          headers: { 'Content-Type': 'application/json' },
          body: JSON.stringify({ graph: this.graph })
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
        alert('Lỗi chạy workflow: ' + err.message);
      }
    }

    startPollingRun(runId) {
      if (this.pollInterval) clearInterval(this.pollInterval);

      this.pollInterval = setInterval(async () => {
        try {
          const resp = await fetch(`/api/flow/runs/${runId}`);
          const res = await resp.json();
          if (!res.ok || !res.run) return;

          const run = res.run;
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
                node.state = nodeState;
                const footer = document.getElementById(`node-footer-${nodeId}`);
                if (footer) footer.innerHTML = this.renderNodeStatus(nodeState);

                const nodeEl = document.getElementById(`node-${nodeId}`);
                if (nodeEl) {
                  nodeEl.classList.toggle('node-running', nodeState.status === 'running');
                  nodeEl.classList.toggle('node-active-executing', nodeState.status === 'running');
                  nodeEl.classList.toggle('node-completed', nodeState.status === 'completed');
                  nodeEl.classList.toggle('node-failed', nodeState.status === 'failed');
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
      const isVideo = filename.endsWith('.mp4') || filename.endsWith('.webm');

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
          <div style="padding:20px;text-align:center">
            ${isVideo ? `<video src="/media/${filename}" controls autoplay style="width:100%;max-height:480px;border-radius:10px;background:#000;box-shadow:0 8px 24px rgba(0,0,0,0.3)"></video>` : `<img src="/media/${filename}" style="max-width:100%;border-radius:10px">`}
            <div style="margin-top:18px;display:flex;gap:12px;justify-content:center">
              <a href="/media/${filename}" download class="wf-btn wf-btn-primary">⬇ Tải video về máy</a>
              <a href="/studio/jobs" class="wf-btn wf-btn-secondary">Xem danh sách Video ↗</a>
            </div>
          </div>
        </div>
      `;
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
      // Close button
      const closeBtn = document.getElementById('btn-close-inspector');
      if (closeBtn) {
        closeBtn.addEventListener('click', () => {
          this.closeInspector();
        });
      }

      // Tab switching
      document.querySelectorAll('.wf-tab').forEach(tab => {
        tab.addEventListener('click', () => {
          document.querySelectorAll('.wf-tab').forEach(t => t.classList.remove('active'));
          document.querySelectorAll('.wf-tab-pane').forEach(c => c.style.display = 'none');
          tab.classList.add('active');
          const target = document.getElementById(`tab-${tab.dataset.tab}`);
          if (target) target.style.display = 'block';
        });
      });

      const clearLogBtn = document.getElementById('btn-clear-logs');
      if (clearLogBtn) {
        clearLogBtn.addEventListener('click', () => this.clearLogs());
      }
    }

    updateInspector() {
      const emptyEl = document.getElementById('inspector-empty');
      const formEl = document.getElementById('inspector-form');
      if (!emptyEl || !formEl) return;

      if (!this.selectedNodeId) {
        emptyEl.style.display = 'block';
        formEl.style.display = 'none';
        return;
      }

      const node = this.graph.nodes.find(n => n.id === this.selectedNodeId);
      if (!node) {
        emptyEl.style.display = 'block';
        formEl.style.display = 'none';
        return;
      }

      emptyEl.style.display = 'none';
      formEl.style.display = 'block';

      const def = this.definitions[node.type] || {};
      const cat = node.category || def.category || 'process';

      const iconEl = document.getElementById('inspector-node-icon');
      if (iconEl) iconEl.innerText = this.getCategoryIcon(cat);

      const titleEl = document.getElementById('inspector-node-title');
      if (titleEl) titleEl.innerText = node.title || def.title || node.type;

      const catEl = document.getElementById('inspector-node-category');
      if (catEl) catEl.innerText = cat.toUpperCase();

      const descEl = document.getElementById('inspector-node-desc');
      if (descEl) descEl.innerText = def.description || '';

      const fieldsContainer = document.getElementById('inspector-fields');
      if (fieldsContainer) {
        fieldsContainer.innerHTML = '';

        if (!node.params) node.params = {};
        const params = Object.assign({}, def.default_params, node.params);

        Object.keys(params).forEach(key => {
          const val = params[key];
          const group = document.createElement('div');
          group.className = 'form-group';

          const label = document.createElement('label');
          label.innerText = this.formatParamLabel(key);
          group.appendChild(label);

          if (typeof val === 'boolean') {
            const chk = document.createElement('input');
            chk.type = 'checkbox';
            chk.checked = val;
            chk.addEventListener('change', () => {
              node.params[key] = chk.checked;
            });
            group.appendChild(chk);
          } else if (typeof val === 'number') {
            const numInput = document.createElement('input');
            numInput.type = 'number';
            numInput.className = 'form-control';
            numInput.value = val;
            numInput.addEventListener('input', () => {
              node.params[key] = parseFloat(numInput.value) || 0;
            });
            group.appendChild(numInput);
          } else if (key.includes('prompt') || key.includes('template') || (typeof val === 'string' && val.length > 50)) {
            const textarea = document.createElement('textarea');
            textarea.className = 'form-control';
            textarea.rows = 4;
            textarea.value = val;
            textarea.addEventListener('input', () => {
              node.params[key] = textarea.value;
            });
            group.appendChild(textarea);
          } else {
            const input = document.createElement('input');
            input.type = 'text';
            input.className = 'form-control';
            input.value = val;
            input.addEventListener('input', () => {
              node.params[key] = input.value;
            });
            group.appendChild(input);
          }

          fieldsContainer.appendChild(group);
        });
      }

      // Preview box
      const previewBox = document.getElementById('inspector-preview-box');
      const previewStatus = document.getElementById('inspector-preview-status');
      if (previewBox) {
        if (node.state && node.state.output_preview) {
          const fn = this.getFilename(node.state.output_preview);
          if (previewStatus) previewStatus.innerText = 'Đã có kết quả';
          if (fn.endsWith('.mp4') || fn.endsWith('.webm')) {
            previewBox.innerHTML = `<video src="/media/${fn}" controls autoplay muted loop style="max-width:100%;border-radius:6px"></video>`;
          } else {
            previewBox.innerHTML = `<img src="/media/${fn}" alt="Preview" style="max-width:100%;border-radius:6px">`;
          }
        } else {
          if (previewStatus) previewStatus.innerText = 'Chưa có kết quả';
          previewBox.innerHTML = '<span class="text-muted">Chưa có dữ liệu xuất</span>';
        }
      }
    }

    formatParamLabel(key) {
      const labels = {
        template: 'Mẫu câu lệnh (Template)',
        prompt: 'Câu lệnh Prompt',
        model_name: 'Tên nhân vật mẫu',
        product_title: 'Tên sản phẩm',
        aspect: 'Tỉ lệ khung hình (Aspect)',
        placement: 'Vị trí ghép sản phẩm',
        scale: 'Tỉ lệ phóng to',
        resolution: 'Độ phân giải',
        expression: 'Biểu cảm & Điệu nhảy',
        provider_name: 'Tên Web AI',
        target_url: 'Đường dẫn đăng nhập AI',
        auto_publish_draft: 'Tự động xuất TikTok nháp'
      };
      return labels[key] || key.replace(/_/g, ' ');
    }

    openAddNodeModal() {
      const modal = document.getElementById('add-node-modal');
      if (modal) {
        modal.style.display = 'flex';
        const searchInput = document.getElementById('node-search-input');
        if (searchInput) {
          searchInput.value = '';
          searchInput.focus();
        }
        this.renderModalGrid('', 'all');
      }
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
          const activeChip = document.querySelector('.wf-tag-filter.active');
          const activeCat = activeChip ? activeChip.dataset.cat : 'all';
          this.renderModalGrid(e.target.value, activeCat);
        });
      }

      document.querySelectorAll('.wf-tag-filter').forEach(chip => {
        chip.addEventListener('click', () => {
          document.querySelectorAll('.wf-tag-filter').forEach(c => c.classList.remove('active'));
          chip.classList.add('active');
          const query = searchInput ? searchInput.value : '';
          this.renderModalGrid(query, chip.dataset.cat);
        });
      });
    }

    renderModalGrid(filterText = '', filterCat = 'all') {
      const grid = document.getElementById('modal-node-grid');
      if (!grid) return;
      grid.innerHTML = '';

      Object.values(this.definitions).forEach(def => {
        if (filterCat !== 'all' && def.category !== filterCat) return;
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
            <div class="wf-card-icon-box cat-${cat}">
              <span>${this.getCategoryIcon(cat)}</span>
            </div>
            <div>
              <h4 class="wf-card-title">${def.title}</h4>
            </div>
          </div>
          <p class="wf-card-desc">${def.description || ''}</p>
          <div class="wf-card-bottom">
            <span class="wf-card-cat-badge">${cat.toUpperCase()}</span>
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
        createBtn.addEventListener('click', () => {
          this.closeGraphsModal();
          this.createNewGraph();
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

        const card = document.createElement('div');
        card.className = `wf-graph-card ${this.graph && this.graph.id === g.id ? 'is-active' : ''}`;

        let catTag = 'tag-custom';
        let catLabel = 'Tự tạo';
        if (g.is_preset) {
          catTag = 'tag-preset';
          catLabel = 'Mẫu Preset';
        } else if (g.category === 'fashion') {
          catTag = 'tag-fashion';
          catLabel = 'Thời trang';
        }

        card.innerHTML = `
          <div>
            <div class="wf-graph-card-header">
              <span class="wf-graph-card-title">${g.name}</span>
              <span class="wf-graph-tag ${catTag}">${catLabel}</span>
            </div>
            <p class="wf-graph-card-desc">${g.description || 'Workflow tự động hóa video'}</p>
          </div>
          <div class="wf-graph-card-meta">
            <span>⚡ ${g.node_count || 0} nodes • ${g.edge_count || 0} dây nối</span>
            <div class="wf-graph-card-actions">
              ${!g.is_preset ? `<button type="button" class="wf-btn wf-btn-danger btn-del-graph" title="Xóa workflow này">🗑️</button>` : ''}
              <button type="button" class="wf-btn wf-btn-primary btn-open-graph">Mở</button>
            </div>
          </div>
        `;

        card.querySelector('.btn-open-graph').addEventListener('click', (e) => {
          e.stopPropagation();
          this.loadGraphById(g.id);
        });

        const delBtn = card.querySelector('.btn-del-graph');
        if (delBtn) {
          delBtn.addEventListener('click', (e) => {
            e.stopPropagation();
            this.deleteGraph(g.id, g.name);
          });
        }

        card.addEventListener('click', () => {
          this.loadGraphById(g.id);
        });

        grid.appendChild(card);
      });
    }

    async loadGraphById(id) {
      try {
        const resp = await fetch(`/api/flow/graphs/${id}`);
        const data = await resp.json();
        if (data.ok && data.graph) {
          this.graph = data.graph;
          if (!this.graph.nodes) this.graph.nodes = [];
          if (!this.graph.edges) this.graph.edges = [];
          this.render();
          this.deselectAll();

          const activeName = document.getElementById('wf-active-name');
          if (activeName) activeName.innerText = this.graph.name;

          const switcher = document.getElementById('wf-switcher-select');
          if (switcher) switcher.value = id;

          window.history.pushState(null, '', `/workflow?preset=${encodeURIComponent(id)}`);
          this.closeGraphsModal();
        } else {
          alert('Không thể mở workflow: ' + (data.error || ''));
        }
      } catch (err) {
        console.error('Lỗi tải workflow:', err);
        alert('Lỗi kết nối máy chủ: ' + err.message);
      }
    }

    createNewGraph() {
      const newId = `flow_${Date.now()}`;
      this.graph = {
        id: newId,
        name: 'Workflow mới tùy chỉnh',
        description: 'Tạo bởi người dùng',
        category: 'custom',
        is_preset: false,
        nodes: [],
        edges: []
      };
      this.render();
      this.deselectAll();

      const activeName = document.getElementById('wf-active-name');
      if (activeName) activeName.innerText = this.graph.name;

      window.history.pushState(null, '', `/workflow?preset=${newId}`);
      this.openAddNodeModal();
    }

    async deleteGraph(id, name) {
      if (!confirm(`Bạn có chắc muốn xóa workflow "${name}"? Thao tác này không thể hoàn tác.`)) {
        return;
      }
      try {
        const resp = await fetch(`/api/flow/graphs/${id}`, { method: 'DELETE' });
        const data = await resp.json();
        if (data.ok) {
          this.savedGraphs = this.savedGraphs.filter(g => g.id !== id);
          await this.refreshGraphsList();
          this.renderGraphsModalGrid();
          if (this.graph.id === id) {
            this.loadGraphById('preset_viral_dance');
          }
        } else {
          alert('Lỗi xóa workflow: ' + (data.error || ''));
        }
      } catch (err) {
        alert('Lỗi kết nối máy chủ: ' + err.message);
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

            switcher.appendChild(presetGroup);
            if (userGroup.children.length > 0) {
              switcher.appendChild(userGroup);
            }
          }
        }
      } catch (e) {
        console.error('Không thể cập nhật danh sách workflow', e);
      }
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