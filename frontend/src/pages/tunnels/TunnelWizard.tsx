import { useMemo, useState } from 'react';
import { useTranslation } from 'react-i18next';
import {
  Alert,
  Button,
  Collapse,
  Descriptions,
  Form,
  Input,
  InputNumber,
  Modal,
  Radio,
  Result,
  Select,
  Space,
  Steps,
  Switch,
  Table,
  Tag,
  Typography,
  message,
} from 'antd';
import { CopyOutlined } from '@ant-design/icons';
import { FormProvider, useWatch } from 'react-hook-form';
import type { Path } from 'react-hook-form';

import { FormField, useZodForm } from '@/components/form/rhf';
import type { CarrierResult, TunnelCapabilities } from '@/generated/zod';
import type { TunnelCreated } from '@/api/queries/useTunnels';
import type { NodeRecord } from '@/schemas/node';
import {
  FAKE_MODES,
  TLS_FINGERPRINTS,
  TUNNEL_CARRIERS,
  tunnelWizardSchema,
  type TunnelWizardValues,
} from '@/schemas/tunnel';
import { defaultWizardValues, kharejSetupCommand, stateColor } from '@/lib/tunnel';
import { panelOrigin } from '@/lib/panelOrigin';
import PortsEditor from './PortsEditor';

interface TunnelWizardProps {
  open: boolean;
  nodes: NodeRecord[];
  capabilities?: TunnelCapabilities;
  creating: boolean;
  onCreate: (values: TunnelWizardValues) => Promise<TunnelCreated | null>;
  onClose: () => void;
}

type Field = Path<TunnelWizardValues>;

const STEP_FIELDS: Field[][] = [
  ['name', 'kharejKind', 'kharejNodeId', 'iranKind', 'iranNodeId'],
  ['ports'],
  ['carrier', 'listenPort', 'path', 'sni', 'reverse', 'fakeSni', 'poolRotate', 'maxMBps'],
  ['token'],
];

const needsPath = (carrier: string) => ['ws', 'wss', 'h2', 'h2c', 'grpc'].includes(carrier);

function EndPicker({
  side,
  nodes,
  methods,
}: {
  side: 'kharej' | 'iran';
  nodes: NodeRecord[];
  methods: ReturnType<typeof useZodForm<TunnelWizardValues>>;
}) {
  const { t } = useTranslation();
  const kind = useWatch({ control: methods.control, name: `${side}Kind` });
  const nodeOptions = useMemo(
    () =>
      nodes
        .filter((n) => n.enable !== false)
        .map((n) => ({ value: n.id, label: n.name || n.address || `#${n.id}` })),
    [nodes],
  );
  return (
    <div className="tunnel-end-pick">
      <Typography.Text strong>{t(`tunnel.role.${side}`)}</Typography.Text>
      <Typography.Text type="secondary">{t(`tunnel.role.${side}Hint`)}</Typography.Text>
      <FormField name={`${side}Kind`}>
        <Radio.Group
          optionType="button"
          buttonStyle="solid"
          options={[
            { value: 'local', label: t('tunnel.host.local') },
            { value: 'node', label: t('tunnel.host.node') },
            { value: 'external', label: t('tunnel.host.external') },
          ]}
        />
      </FormField>
      {kind === 'node' && (
        <FormField name={`${side}NodeId`} required>
          <Select
            showSearch
            options={nodeOptions}
            placeholder={t('tunnel.pickNode')}
            optionFilterProp="label"
            notFoundContent={t('tunnel.noNodes')}
          />
        </FormField>
      )}
      {kind === 'external' && (
        <Typography.Text type="secondary">{t('tunnel.host.externalHint')}</Typography.Text>
      )}
      <FormField name={`${side}Address`} extra={t('tunnel.addressHint')}>
        <Input placeholder={t('tunnel.addressPlaceholder')} style={{ direction: 'ltr' }} />
      </FormField>
    </div>
  );
}

export default function TunnelWizard(props: TunnelWizardProps) {
  // Closed means unmounted, so every opening starts from a fresh form.
  return props.open ? <WizardModal {...props} /> : null;
}

function WizardModal({
  open,
  nodes,
  capabilities,
  creating,
  onCreate,
  onClose,
}: TunnelWizardProps) {
  const { t } = useTranslation();
  const [messageApi, messageContextHolder] = message.useMessage();
  const methods = useZodForm<TunnelWizardValues>(tunnelWizardSchema, {
    defaultValues: defaultWizardValues(),
  });
  const [step, setStep] = useState(0);
  const [created, setCreated] = useState<TunnelCreated | null>(null);

  const v = useWatch({ control: methods.control }) as TunnelWizardValues;
  const origin = panelOrigin();
  const externalKharej = v.kharejKind === 'external';

  const next = async () => {
    if (await methods.trigger(STEP_FIELDS[step])) setStep(step + 1);
  };

  const submit = methods.handleSubmit(async (values) => {
    const res = await onCreate(values);
    if (res) setCreated(res);
  });

  const copy = async (text: string) => {
    try {
      await navigator.clipboard.writeText(text);
      messageApi.success(t('copied'));
    } catch {
      messageApi.error(t('somethingWentWrong'));
    }
  };

  const steps = [
    { title: t('tunnel.step.ends') },
    { title: t('tunnel.step.ports') },
    { title: t('tunnel.step.connection') },
    { title: t('tunnel.step.review') },
  ];

  const carrierOptions = [
    { value: 'auto', label: t('tunnel.carrierAuto') },
    ...TUNNEL_CARRIERS.filter((c) => capabilities?.carriers.includes(c) ?? true).map((c) => ({
      value: c,
      label: c,
    })),
  ];

  const probeColumns = [
    { title: t('tunnel.carrier'), dataIndex: 'carrier' },
    {
      title: t('tunnel.state.title'),
      dataIndex: 'ok',
      render: (ok: boolean, r: CarrierResult) =>
        ok ? (
          <Tag color="success">{t('tunnel.state.up')}</Tag>
        ) : (
          <Tag>{r.why || t('tunnel.state.down')}</Tag>
        ),
    },
    {
      title: t('tunnel.speed'),
      render: (_: unknown, r: CarrierResult) =>
        r.ok ? `${r.downMBps.toFixed(1)} / ${r.upMBps.toFixed(1)} MB/s` : '-',
    },
  ];

  const body = created ? (
    <Space orientation="vertical" size="middle" style={{ width: '100%' }}>
      <Result
        status={created.tunnel?.state === 'up' ? 'success' : 'info'}
        title={t('tunnel.created.title', { name: created.tunnel?.name ?? '' })}
        subTitle={
          <Tag color={stateColor(created.tunnel?.state ?? 'unknown')}>
            {t(`tunnel.state.${created.tunnel?.state ?? 'unknown'}`)}
          </Tag>
        }
      />
      {created.tunnel && created.tunnel.probe.length > 0 && (
        <Table
          size="small"
          pagination={false}
          rowKey="carrier"
          dataSource={created.tunnel.probe}
          columns={probeColumns}
        />
      )}
      {created.access && (
        <>
          <Alert type="warning" showIcon title={t('tunnel.created.externalIran')} />
          <Input.TextArea
            readOnly
            rows={4}
            value={created.access.command}
            className="tunnel-command"
          />
          <Button icon={<CopyOutlined />} onClick={() => copy(created.access!.command)}>
            {t('copy')}
          </Button>
        </>
      )}
    </Space>
  ) : (
    <FormProvider {...methods}>
      <Form layout="vertical" onFinish={submit}>
        {step === 0 && (
          <>
            <FormField name="name" label={t('tunnel.name')} required>
              <Input maxLength={63} />
            </FormField>
            <Space orientation="vertical" size="middle" style={{ width: '100%' }}>
              <EndPicker side="kharej" nodes={nodes} methods={methods} />
              <EndPicker side="iran" nodes={nodes} methods={methods} />
            </Space>
          </>
        )}
        {step === 1 && (
          <>
            <Typography.Paragraph type="secondary">{t('tunnel.ports.intro')}</Typography.Paragraph>
            <FormField name="ports">
              <PortsEditor />
            </FormField>
          </>
        )}
        {step === 2 && (
          <>
            <FormField name="carrier" label={t('tunnel.carrier')} extra={t('tunnel.carrierHint')}>
              <Select options={carrierOptions} />
            </FormField>
            <Collapse
              ghost
              items={[
                {
                  key: 'advanced',
                  label: t('tunnel.advanced'),
                  children: (
                    <>
                      {needsPath(v.carrier) && (
                        <FormField name="path" label={t('tunnel.path')}>
                          <Input placeholder="/wat" style={{ direction: 'ltr' }} />
                        </FormField>
                      )}
                      {['tls', 'wss', 'h2', 'grpc'].includes(v.carrier) && (
                        <FormField name="sni" label="SNI">
                          <Input style={{ direction: 'ltr' }} />
                        </FormField>
                      )}
                      <FormField
                        name="listenPort"
                        label={t('tunnel.listenPort')}
                        extra={t('tunnel.listenPortHint')}
                      >
                        <InputNumber min={0} max={65535} precision={0} style={{ width: '100%' }} />
                      </FormField>
                      <FormField name="maxMBps" label={t('tunnel.maxSpeed')}>
                        <InputNumber min={0} precision={0} style={{ width: '100%' }} />
                      </FormField>
                      <FormField
                        name="reverse"
                        label={t('tunnel.reverse')}
                        extra={t('tunnel.reverseHint')}
                        valueProp="checked"
                      >
                        <Switch />
                      </FormField>
                      <Typography.Title level={5}>{t('tunnel.kit.title')}</Typography.Title>
                      <Alert
                        type="info"
                        showIcon
                        style={{ marginBottom: 12 }}
                        title={t('tunnel.kit.hint')}
                      />
                      <FormField name="fakeSni" label={t('tunnel.kit.fakeSni')}>
                        <Input placeholder="www.cloudflare.com" style={{ direction: 'ltr' }} />
                      </FormField>
                      <FormField name="fakeMode" label={t('tunnel.kit.fakeMode')}>
                        <Select
                          allowClear
                          options={FAKE_MODES.map((m) => ({ value: m, label: m }))}
                          placeholder="seq"
                        />
                      </FormField>
                      <FormField name="poolRotate" label={t('tunnel.kit.poolRotate')}>
                        <Input placeholder="30m" style={{ direction: 'ltr' }} />
                      </FormField>
                      <FormField name="ech" label="ECH">
                        <Select
                          allowClear
                          options={['auto', 'ladder'].map((m) => ({ value: m, label: m }))}
                        />
                      </FormField>
                      <FormField name="fp" label={t('tunnel.kit.fingerprint')}>
                        <Select
                          allowClear
                          options={TLS_FINGERPRINTS.map((m) => ({ value: m, label: m }))}
                        />
                      </FormField>
                      <FormField
                        name="tlsSmallCh"
                        label={t('tunnel.kit.smallHello')}
                        valueProp="checked"
                      >
                        <Switch />
                      </FormField>
                    </>
                  ),
                },
              ]}
            />
          </>
        )}
        {step === 3 && (
          <Space orientation="vertical" size="middle" style={{ width: '100%' }}>
            <Descriptions
              size="small"
              bordered
              column={1}
              items={[
                { key: 'n', label: t('tunnel.name'), children: v.name },
                {
                  key: 'k',
                  label: t('tunnel.role.kharej'),
                  children: endLabel(v.kharejKind, v.kharejNodeId, nodes, t),
                },
                {
                  key: 'i',
                  label: t('tunnel.role.iran'),
                  children: endLabel(v.iranKind, v.iranNodeId, nodes, t),
                },
                { key: 'c', label: t('tunnel.carrier'), children: v.carrier },
                {
                  key: 'p',
                  label: t('tunnel.portsTitle'),
                  children: v.ports
                    .map(
                      (p) =>
                        `${p.listen}${p.target !== p.listen ? `→${p.target}` : ''}${p.udp ? '/udp' : ''}`,
                    )
                    .join(', '),
                },
              ]}
            />
            {externalKharej && (
              <>
                <Alert type="info" showIcon title={t('tunnel.externalKharej.steps')} />
                <Input.TextArea
                  readOnly
                  rows={3}
                  className="tunnel-command"
                  value={kharejSetupCommand(origin, v)}
                />
                <Button icon={<CopyOutlined />} onClick={() => copy(kharejSetupCommand(origin, v))}>
                  {t('copy')}
                </Button>
                <FormField name="token" label={t('tunnel.externalKharej.token')} required>
                  <Input.TextArea rows={3} className="tunnel-command" placeholder="wat1:..." />
                </FormField>
              </>
            )}
            {capabilities && !capabilities.binaryAvailable && (
              <Alert type="warning" showIcon title={t('tunnel.noBinary')} />
            )}
          </Space>
        )}
      </Form>
    </FormProvider>
  );

  const footer = created ? (
    <Button type="primary" onClick={onClose}>
      {t('close')}
    </Button>
  ) : (
    <Space>
      <Button onClick={onClose}>{t('cancel')}</Button>
      {step > 0 && <Button onClick={() => setStep(step - 1)}>{t('tunnel.back')}</Button>}
      {step < steps.length - 1 ? (
        <Button type="primary" onClick={next}>
          {t('tunnel.next')}
        </Button>
      ) : (
        <Button type="primary" loading={creating} onClick={submit}>
          {t('create')}
        </Button>
      )}
    </Space>
  );

  return (
    <Modal
      open={open}
      title={t('tunnel.add')}
      width={720}
      onCancel={onClose}
      footer={footer}
      mask={{ closable: false }}
      destroyOnHidden
    >
      {messageContextHolder}
      {!created && <Steps current={step} items={steps} size="small" style={{ marginBottom: 20 }} />}
      {body}
    </Modal>
  );
}

function endLabel(
  kind: string,
  nodeId: number | null,
  nodes: NodeRecord[],
  t: (key: string) => string,
): string {
  if (kind === 'node') return nodes.find((n) => n.id === nodeId)?.name ?? `#${nodeId}`;
  return t(`tunnel.host.${kind}`);
}
