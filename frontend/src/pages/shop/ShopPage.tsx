import { useCallback, useEffect, useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Card, ConfigProvider, Layout, Spin, Typography } from 'antd';

import type { ShopCatalog, ShopOrderView, ShopSession } from '@/generated/zod';
import { launchInitData } from '@/lib/telegramApp';
import { errorKey, setShopSession, shopApi } from '@/lib/shopApi';
import { useTheme } from '@/hooks/useTheme';
import CheckoutForm from './CheckoutForm';
import CustomPlan from './CustomPlan';
import OrderStatus from './OrderStatus';
import PlanList from './PlanList';
import ServicesList from './ServicesList';
import SupportTickets from './SupportTickets';
import type { Offer, Preset } from './offer';
import './ShopPage.css';

const POLL_MS = 5000;

function tokenFromUrl(): string {
  return new URLSearchParams(window.location.search).get('order') ?? '';
}

function isSettling(order: ShopOrderView): boolean {
  if (order.status === 'pending' || order.status === 'review') return true;
  return order.status === 'paid' && !order.fulfilled;
}

export default function ShopPage() {
  const { t } = useTranslation();
  const { antdThemeConfig } = useTheme();
  const [catalog, setCatalog] = useState<ShopCatalog | null>(null);
  const [loadFailed, setLoadFailed] = useState(false);
  const [offer, setOffer] = useState<Offer | null>(null);
  const [preset, setPreset] = useState<Preset | null>(null);
  const [session, setSession] = useState<ShopSession | null>(null);
  const [sessionError, setSessionError] = useState('');
  const [order, setOrder] = useState<ShopOrderView | null>(null);
  const [booting, setBooting] = useState(true);

  const show = useCallback((view: ShopOrderView) => {
    setOrder(view);
    const url = new URL(window.location.href);
    url.searchParams.set('order', view.token);
    window.history.replaceState(null, '', url);
  }, []);

  useEffect(() => {
    let cancelled = false;
    void (async () => {
      const token = tokenFromUrl();
      const initData = launchInitData(window.location.hash);
      // Sign in first: the catalog and the order calls then carry the session.
      const signedIn = initData ? await shopApi.telegramSession(initData) : null;
      if (signedIn?.ok) {
        setShopSession(signedIn.data.token);
        if (!cancelled) setSession(signedIn.data);
      } else if (signedIn && !cancelled) {
        setSessionError(errorKey(signedIn.code));
      }
      const [plans, existing] = await Promise.all([
        shopApi.catalog(),
        token ? shopApi.order(token) : Promise.resolve(null),
      ]);
      if (cancelled) return;
      if (plans.ok) setCatalog(plans.data);
      else setLoadFailed(true);
      if (existing?.ok) setOrder(existing.data);
      setBooting(false);
    })();
    return () => {
      cancelled = true;
    };
  }, []);

  const token = order?.token;
  const settling = order ? isSettling(order) : false;
  useEffect(() => {
    if (!token || !settling) return undefined;
    const timer = window.setInterval(() => {
      void shopApi.order(token).then((res) => {
        if (res.ok) setOrder(res.data);
      });
    }, POLL_MS);
    return () => window.clearInterval(timer);
  }, [token, settling]);

  const selectPlan = (id: number) => {
    const plan = catalog?.plans.find((p) => p.id === id);
    if (plan)
      setOffer({
        planId: plan.id,
        name: plan.name,
        price: plan.price,
        currency: plan.currency,
      });
  };

  const renew = (email: string) => {
    setPreset({ kind: 'renew', email });
    setOffer(null);
  };
  const addon = (email: string) => {
    setPreset({ kind: 'addon', email });
    setOffer(null);
  };

  const adding = preset?.kind === 'addon';
  const planSelected = offer && offer.planId > 0 ? offer.planId : null;

  return (
    <ConfigProvider theme={antdThemeConfig}>
      <Layout className="shop-page">
        <Layout.Content className="shop-content">
          <Typography.Title level={3} className="shop-title">
            {t('shop.title')}
          </Typography.Title>
          {booting && <Spin />}
          {!booting && loadFailed && (
            <Alert type="error" showIcon title={t('shop.errors.network')} />
          )}
          {!booting && sessionError && (
            <Alert
              type="warning"
              showIcon
              title={
                sessionError === 'shop.errors.proof_invalid'
                  ? t('shopbot.app.sessionExpired')
                  : t(sessionError)
              }
              style={{ marginBottom: 12 }}
            />
          )}
          {!booting && session && (
            <>
              <Typography.Paragraph dir="auto">
                {t('shopbot.app.signedIn', { name: session.name })}
              </Typography.Paragraph>
              <ServicesList
                services={session.services}
                onRenew={renew}
                onAddon={catalog?.custom ? addon : undefined}
              />
            </>
          )}
          {!booting && order && (
            <Card size="small">
              <OrderStatus order={order} onChange={setOrder} />
            </Card>
          )}
          {!booting && !order && catalog && (
            <>
              {preset && (
                <Alert
                  type="info"
                  showIcon
                  closable
                  onClose={() => {
                    setPreset(null);
                    setOffer(null);
                  }}
                  title={t(
                    preset.kind === 'renew' ? 'shopbot.app.renewing' : 'shopbot.app.adding',
                    {
                      email: preset.email,
                    },
                  )}
                  style={{ margin: '12px 0' }}
                />
              )}
              {!adding && (
                <PlanList plans={catalog.plans} selected={planSelected} onSelect={selectPlan} />
              )}
              {catalog.custom && !offer?.custom && (
                <CustomPlan custom={catalog.custom} addon={adding} onSelect={setOffer} />
              )}
              {offer && (
                <Card size="small" className="shop-checkout">
                  <CheckoutForm
                    key={`${offer.planId}:${offer.custom?.gb ?? 0}:${offer.custom?.days ?? 0}:${preset?.email ?? ''}`}
                    offer={offer}
                    methods={catalog.methods}
                    session={session}
                    preset={preset}
                    onOrder={show}
                  />
                </Card>
              )}
            </>
          )}
          {!booting && (
            <SupportTickets orderToken={order?.token ?? ''} services={session?.services} />
          )}
        </Layout.Content>
      </Layout>
    </ConfigProvider>
  );
}
