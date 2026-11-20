import clsx from 'clsx';
import type { JSX } from 'solid-js';

export interface IconProps {
  class?: string;
  style?: JSX.CSSProperties;
  title?: string;
  spin?: boolean;
}

function makeIcon(icon: string) {
  return function Icon(props: IconProps) {
    return (
      <span
        class={clsx(
          'inline-block shrink-0 align-middle',
          icon,
          props.spin && 'animate-spin',
          props.class ?? 'h-4 w-4',
        )}
        style={props.style}
        title={props.title}
        aria-hidden={props.title ? undefined : 'true'}
      />
    );
  };
}

/* Fluent 图标字面量：Tailwind 在源码中直接看到类名，避免动态拼接不生成。 */
const ICON = {
  api: 'icon-[fluent--plug-connected-20-regular]',
  arrowLeft: 'icon-[fluent--arrow-left-20-regular]',
  arrowRight: 'icon-[fluent--arrow-right-20-regular]',
  branches: 'icon-[fluent--branch-20-regular]',
  check: 'icon-[fluent--checkmark-20-regular]',
  clock: 'icon-[fluent--clock-20-regular]',
  cloudDownload: 'icon-[fluent--cloud-arrow-down-20-regular]',
  cluster: 'icon-[fluent--box-multiple-20-regular]',
  code: 'icon-[fluent--code-20-regular]',
  copy: 'icon-[fluent--copy-20-regular]',
  dashboard: 'icon-[fluent--data-pie-20-regular]',
  delete: 'icon-[fluent--delete-20-regular]',
  deployment: 'icon-[fluent--server-20-regular]',
  desktop: 'icon-[fluent--desktop-20-regular]',
  down: 'icon-[fluent--chevron-down-20-regular]',
  edit: 'icon-[fluent--edit-20-regular]',
  eye: 'icon-[fluent--eye-20-regular]',
  eyeOff: 'icon-[fluent--eye-off-20-regular]',
  fileSearch: 'icon-[fluent--document-search-20-regular]',
  info: 'icon-[fluent--info-20-regular]',
  key: 'icon-[fluent--key-20-regular]',
  kubernetes: 'icon-[fluent--box-multiple-20-regular]',
  lock: 'icon-[fluent--lock-closed-20-regular]',
  login: 'icon-[fluent--arrow-enter-20-regular]',
  logout: 'icon-[fluent--sign-out-20-regular]',
  play: 'icon-[fluent--play-circle-20-regular]',
  plus: 'icon-[fluent--add-20-regular]',
  reload: 'icon-[fluent--arrow-clockwise-20-regular]',
  robot: 'icon-[fluent--bot-20-regular]',
  safety: 'icon-[fluent--shield-20-regular]',
  save: 'icon-[fluent--save-20-regular]',
  search: 'icon-[fluent--search-20-regular]',
  setting: 'icon-[fluent--settings-20-regular]',
  team: 'icon-[fluent--people-20-regular]',
  thunderbolt: 'icon-[fluent--flash-20-regular]',
  upload: 'icon-[fluent--arrow-upload-20-regular]',
  userAdd: 'icon-[fluent--person-add-20-regular]',
  user: 'icon-[fluent--person-20-regular]',
  dismiss: 'icon-[fluent--dismiss-20-regular]',
  filter: 'icon-[fluent--filter-20-regular]',
  chevronUp: 'icon-[fluent--chevron-up-20-regular]',
  calendar: 'icon-[fluent--calendar-ltr-20-regular]',
  spinner: 'icon-[fluent--spinner-ios-16-regular]',
  infoSmall: 'icon-[fluent--info-16-regular]',
  warningSmall: 'icon-[fluent--warning-16-regular]',
  errorSmall: 'icon-[fluent--error-circle-16-regular]',
  successSmall: 'icon-[fluent--checkmark-circle-16-regular]',
} as const;

export const ApiOutlined = makeIcon(ICON.api);
export const ArrowLeftOutlined = makeIcon(ICON.arrowLeft);
export const ArrowRightOutlined = makeIcon(ICON.arrowRight);
export const BranchesOutlined = makeIcon(ICON.branches);
export const CheckOutlined = makeIcon(ICON.check);
export const ClockCircleOutlined = makeIcon(ICON.clock);
export const CloudDownloadOutlined = makeIcon(ICON.cloudDownload);
export const ClusterOutlined = makeIcon(ICON.cluster);
export const CodeOutlined = makeIcon(ICON.code);
export const CopyOutlined = makeIcon(ICON.copy);
export const DashboardOutlined = makeIcon(ICON.dashboard);
export const DeleteOutlined = makeIcon(ICON.delete);
export const DeploymentUnitOutlined = makeIcon(ICON.deployment);
export const DesktopOutlined = makeIcon(ICON.desktop);
export const DownOutlined = makeIcon(ICON.down);
export const EditOutlined = makeIcon(ICON.edit);
export const EyeOutlined = makeIcon(ICON.eye);
export const FileSearchOutlined = makeIcon(ICON.fileSearch);
export const InfoCircleOutlined = makeIcon(ICON.info);
export const KeyOutlined = makeIcon(ICON.key);
export const KubernetesOutlined = makeIcon(ICON.kubernetes);
export const LockOutlined = makeIcon(ICON.lock);
export const LoginOutlined = makeIcon(ICON.login);
export const LogoutOutlined = makeIcon(ICON.logout);
export const PlayCircleOutlined = makeIcon(ICON.play);
export const PlusOutlined = makeIcon(ICON.plus);
export const ReloadOutlined = makeIcon(ICON.reload);
export const RobotOutlined = makeIcon(ICON.robot);
export const SafetyCertificateOutlined = makeIcon(ICON.safety);
export const SaveOutlined = makeIcon(ICON.save);
export const SearchOutlined = makeIcon(ICON.search);
export const SettingOutlined = makeIcon(ICON.setting);
export const TeamOutlined = makeIcon(ICON.team);
export const ThunderboltOutlined = makeIcon(ICON.thunderbolt);
export const UploadOutlined = makeIcon(ICON.upload);
export const UserAddOutlined = makeIcon(ICON.userAdd);
export const UserOutlined = makeIcon(ICON.user);

export const DismissIcon = makeIcon(ICON.dismiss);
export const FilterIcon = makeIcon(ICON.filter);
export const ChevronUpIcon = makeIcon(ICON.chevronUp);
export const CalendarIcon = makeIcon(ICON.calendar);
export const SpinnerIcon = makeIcon(ICON.spinner);
export const EyeOffIcon = makeIcon(ICON.eyeOff);
export const InfoIcon = makeIcon(ICON.infoSmall);
export const WarningIcon = makeIcon(ICON.warningSmall);
export const ErrorIcon = makeIcon(ICON.errorSmall);
export const SuccessIcon = makeIcon(ICON.successSmall);
