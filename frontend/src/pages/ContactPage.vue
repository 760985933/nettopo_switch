<script setup lang="ts">
import { useI18n } from 'vue-i18n'
import { BrowserOpenURL } from '../../wailsjs/runtime/runtime'
import contactImg from '../assets/images/contact.png'
import wxImg from '../assets/images/wx.png'
import zfbImg from '../assets/images/zfb.png'

const { t } = useI18n()

const sponsors = [
  { name: 'NetTopo专业网络运维软件', url: 'https://www.nettopo.com' },
  { name: 'AAA专业批发空气小李', url: '' },
]
</script>

<template>
  <div class="contact-page">
    <div class="page-head">
      <div>
        <h2>{{ t('contact.title') }}</h2>
        <p>{{ t('contact.desc') }}</p>
      </div>
    </div>

    <div class="contact-grid">
      <div class="card">
        <h4 class="card-title">{{ t('contact.contactLabel') }}</h4>
        <div class="img-wrap">
          <img :src="contactImg" alt="contact" class="contact-img" />
        </div>
      </div>

      <div class="card">
        <h4 class="card-title">{{ t('contact.sponsorLabel') }}</h4>
        <div class="sponsor-imgs">
          <div class="sponsor-item">
            <img :src="wxImg" alt="WeChat" class="sponsor-img" />
            <span class="sponsor-label">{{ t('contact.wxLabel') }}</span>
          </div>
          <div class="sponsor-item">
            <img :src="zfbImg" alt="Alipay" class="sponsor-img" />
            <span class="sponsor-label">{{ t('contact.zfbLabel') }}</span>
          </div>
        </div>
      </div>
    </div>

    <div class="card sponsor-list-card">
      <h4 class="card-title">{{ t('contact.sponsorList') }}</h4>
      <div class="sponsor-list">
        <div v-for="s in sponsors" :key="s.name" class="sponsor-row">
          <span class="sponsor-name">{{ s.name }}</span>
          <a class="sponsor-url" @click="BrowserOpenURL(s.url)">{{ s.url }}</a>
        </div>
      </div>
    </div>
  </div>
</template>

<style scoped>
.contact-page {
  display: grid;
  gap: 14px;
}

.page-head {
  padding: 18px;
  border-radius: 22px;
  border: 1px solid var(--border);
  background: var(--surface);
  box-shadow: 0 10px 30px rgba(14, 30, 68, 0.08);
}

.page-head h2 {
  margin: 0 0 6px;
  font-size: 18px;
  color: var(--text);
}

.page-head p {
  margin: 0;
  font-size: 12px;
  color: var(--muted);
}

.contact-grid {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 14px;
}

.card {
  padding: 18px;
  border-radius: 22px;
  border: 1px solid var(--border);
  background: var(--surface);
  box-shadow: 0 10px 30px rgba(14, 30, 68, 0.08);
  display: grid;
  gap: 14px;
}

.card-title {
  margin: 0;
  font-size: 14px;
  color: var(--text);
}

.img-wrap {
  display: flex;
  justify-content: center;
  max-width: 300px;
  margin: 0 auto;
}

.contact-img {
  width: 100%;
  height: auto;
  max-width: 100%;
  object-fit: contain;
  border-radius: 12px;
}

.sponsor-imgs {
  display: grid;
  grid-template-columns: 1fr 1fr;
  gap: 14px;
}

.sponsor-item {
  display: grid;
  gap: 8px;
  text-align: center;
}

.sponsor-img {
  width: 100%;
  height: auto;
  border-radius: 12px;
}

.sponsor-label {
  font-size: 12px;
  color: var(--muted);
}

.sponsor-list-card {
  display: grid;
  gap: 12px;
}

.sponsor-list {
  display: grid;
  gap: 10px;
}

.sponsor-row {
  display: flex;
  align-items: center;
  gap: 12px;
  padding: 10px 12px;
  border-radius: 14px;
  border: 1px solid var(--border);
  background: rgba(255, 255, 255, 0.82);
}

.sponsor-name {
  font-size: 14px;
  font-weight: 600;
  color: var(--text);
}

.sponsor-url {
  font-size: 12px;
  color: var(--accent);
  cursor: pointer;
  text-decoration: none;
}

.sponsor-url:hover {
  text-decoration: underline;
}

@media (max-width: 720px) {
  .contact-grid {
    grid-template-columns: 1fr;
  }

  .sponsor-imgs {
    grid-template-columns: 1fr 1fr;
  }
}
</style>
