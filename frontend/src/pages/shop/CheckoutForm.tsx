import { useState } from 'react';
import { useTranslation } from 'react-i18next';
import { Alert, Button, Input, Radio, Select, Space, Typography } from 'antd';

import type { ShopOrderView, ShopSession } from '@/generated/zod';
import { formatMoney } from '@/lib/money';
import { errorKey, shopApi, type OrderInput } from '@/lib/shopApi';
import type { Offer, Preset } from './offer';

type Kind = OrderInput['kind'];

interface CheckoutFormProps {
  offer: Offer;
  methods: string[];
  session: ShopSession | null;
  preset: Preset | null;
  onOrder: (view: ShopOrderView) => void;
}

export default function CheckoutForm({
  offer,
  methods,
  session,
  preset,
  onOrder,
}: CheckoutFormProps) {
  const { t } = useTranslation();
  const kinds: Kind[] = preset
    ? [preset.kind]
    : offer.custom
      ? ['new', 'renew', 'addon']
      : ['new', 'renew'];
  const [kind, setKind] = useState<Kind>(kinds[0]);
  const [email, setEmail] = useState(preset?.email ?? '');
  const [contact, setContact] = useState('');
  const [discountCode, setDiscountCode] = useState('');
  const [referralCode, setReferralCode] = useState('');
  const [proof, setProof] = useState('');
  const [gateway, setGateway] = useState(methods[0] ?? '');
  const [final, setFinal] = useState<number | null>(null);
  const [error, setError] = useState('');
  const [busy, setBusy] = useState(false);

  const ownsAccount = session?.services.some((s) => s.email === email) ?? false;
  const wallet = gateway === 'wallet';
  const methodOptions = methods
    .filter((m) => m !== 'wallet' || kind !== 'new')
    .map((m) => ({
      value: m,
      label: t(`pages.sales.gateways.${m}`, { defaultValue: m }),
    }));
  const sizeBody = offer.custom
    ? { customGb: offer.custom.gb, customDays: offer.custom.days }
    : undefined;
  const kindLabel: Record<Kind, string> = {
    new: t('shop.newAccount'),
    renew: t('shop.renewAccount'),
    addon: t('shopbot.app.addonAccount'),
  };

  async function applyCode() {
    setError('');
    setFinal(null);
    if (!discountCode.trim()) return;
    const res = await shopApi.quote({
      planId: offer.planId,
      kind,
      ...sizeBody,
      discountCode,
      email,
    });
    if (res.ok) setFinal(res.data.final);
    else setError(errorKey(res.code));
  }

  async function submit() {
    setBusy(true);
    setError('');
    const res = await shopApi.createOrder({
      planId: offer.planId,
      ...sizeBody,
      kind,
      email,
      gateway,
      contact,
      discountCode,
      referralCode: kind === 'new' ? referralCode : '',
      proof: wallet && !ownsAccount ? proof : '',
    });
    setBusy(false);
    if (!res.ok) {
      setError(errorKey(res.code));
      return;
    }
    onOrder(res.data);
    if (res.data.payUrl) window.location.assign(res.data.payUrl);
  }

  if (methods.length === 0) return <Alert type="warning" showIcon title={t('shop.noMethods')} />;

  const pickFromServices = session != null && kind !== 'new' && !preset;
  return (
    <Space direction="vertical" size={12} style={{ width: '100%' }}>
      <Typography.Text strong dir="auto">
        {offer.name}: {formatMoney(final ?? offer.price, offer.currency)}
      </Typography.Text>
      {kinds.length > 1 && (
        <Radio.Group
          value={kind}
          onChange={(e) => {
            const next = e.target.value as Kind;
            setKind(next);
            setFinal(null);
            if (next === 'new' && gateway === 'wallet')
              setGateway(methods.find((m) => m !== 'wallet') ?? '');
          }}
        >
          {kinds.map((k) => (
            <Radio.Button key={k} value={k}>
              {kindLabel[k]}
            </Radio.Button>
          ))}
        </Radio.Group>
      )}
      {pickFromServices ? (
        <Select
          value={email || undefined}
          placeholder={t('shopbot.app.pickService')}
          options={session.services.map((s) => ({
            value: s.email,
            label: s.email,
          }))}
          onChange={setEmail}
        />
      ) : (
        <Input
          dir="ltr"
          value={email}
          maxLength={64}
          disabled={preset != null}
          placeholder={kind === 'new' ? t('shop.chooseUsername') : t('shop.yourUsername')}
          onChange={(e) => setEmail(e.target.value)}
        />
      )}
      <Input
        dir="auto"
        value={contact}
        maxLength={100}
        placeholder={t('shop.contact')}
        onChange={(e) => setContact(e.target.value)}
      />
      <Input.Search
        dir="ltr"
        value={discountCode}
        placeholder={t('shop.discountCode')}
        enterButton={t('shop.apply')}
        onChange={(e) => {
          setDiscountCode(e.target.value);
          setFinal(null);
        }}
        onSearch={applyCode}
      />
      {kind === 'new' && (
        <Input
          dir="ltr"
          value={referralCode}
          maxLength={16}
          placeholder={t('shop.referralCode')}
          onChange={(e) => setReferralCode(e.target.value)}
        />
      )}
      <Select
        value={gateway}
        options={methodOptions}
        style={{ width: '100%' }}
        onChange={setGateway}
        aria-label={t('shop.payWith')}
      />
      {wallet && !ownsAccount && (
        <Input.Password
          dir="ltr"
          value={proof}
          placeholder={t('shop.subscriptionId')}
          onChange={(e) => setProof(e.target.value)}
        />
      )}
      {error && <Alert type="error" showIcon title={t(error)} />}
      <Button
        type="primary"
        block
        loading={busy}
        disabled={(!email.trim() && !(session && kind === 'new')) || !gateway}
        onClick={submit}
      >
        {t('shop.buy')}
      </Button>
    </Space>
  );
}
